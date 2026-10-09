package dev.lavalink.youtube.track;

import com.sedmelluq.discord.lavaplayer.tools.http.HttpContextFilter;
import com.sedmelluq.discord.lavaplayer.tools.io.HttpInterface;
import com.sedmelluq.discord.lavaplayer.track.AudioTrackInfo;
import com.sun.net.httpserver.HttpServer;
import dev.lavalink.youtube.clients.skeleton.Client;
import dev.lavalink.youtube.track.format.StreamFormat;
import dev.lavalink.youtube.track.format.TrackFormats;
import org.apache.http.client.config.RequestConfig;
import org.apache.http.client.protocol.HttpClientContext;
import org.apache.http.entity.ContentType;
import org.apache.http.impl.client.CloseableHttpClient;
import org.apache.http.impl.client.HttpClients;
import org.apache.http.impl.conn.PoolingHttpClientConnectionManager;
import org.apache.http.config.RegistryBuilder;
import org.apache.http.conn.socket.ConnectionSocketFactory;
import org.apache.http.conn.socket.PlainConnectionSocketFactory;

import java.lang.reflect.Field;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.URI;
import java.util.Collections;
import java.util.concurrent.atomic.AtomicInteger;

/** Exercises the real URL-selection method and real HTTP requests, without YouTube or Discord. */
public final class CdnFallbackTest {
  public static void main(String[] args) throws Exception {
    AtomicInteger primaryRequests = new AtomicInteger();
    AtomicInteger alternateRequests = new AtomicInteger();
    HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    server.createContext("/audio", exchange -> {
      boolean alternate = exchange.getRequestHeaders().getFirst("Host").contains("sn-backup");
      if (alternate) alternateRequests.incrementAndGet(); else primaryRequests.incrementAndGet();
      String query = exchange.getRequestURI().getRawQuery();
      boolean success = !query.contains("allfail=1") && (alternate || query.contains("healthy=1"));
      if (success) {
        exchange.getResponseHeaders().set("Content-Range", "bytes 0-0/1000000");
        exchange.sendResponseHeaders(206, 1);
        exchange.getResponseBody().write(0);
      } else exchange.sendResponseHeaders(503, -1);
      exchange.close();
    });
    server.start();
    try {
      String query = "mn=sn-primary%2Csn-backup&sig=a%2Bb%2Fc%3D&n=unchanged";
      URI primary = URI.create("http://rr1---sn-primary.googlevideo.com:" + server.getAddress().getPort() + "/audio?" + query);
      URI resolved = select(primary, false);
      check(resolved.getHost().equals("rr1---sn-backup.googlevideo.com"), "failed primary CDN must select the advertised alternate; got " + resolved.getHost());
      check(resolved.getRawQuery().equals(primary.getRawQuery()), "fallback must preserve the signed query byte for byte");
      check(primaryRequests.get() == 1 && alternateRequests.get() == 1, "probe must try primary and alternate once");
      System.out.println("PASS: HTTP failure selects advertised alternate and preserves signature");

      primaryRequests.set(0); alternateRequests.set(0);
      resolved = select(primary, true);
      check(resolved.getHost().equals("rr1---sn-backup.googlevideo.com"), "connection refusal must select advertised alternate");
      check(primaryRequests.get() == 0 && alternateRequests.get() == 1, "transport failure must make only one alternate request");
      System.out.println("PASS: transport failure selects advertised alternate");

      primaryRequests.set(0); alternateRequests.set(0);
      URI healthy = URI.create(primary + "&healthy=1");
      check(select(healthy, false).equals(healthy), "healthy primary must remain selected");
      check(primaryRequests.get() == 1 && alternateRequests.get() == 0, "healthy primary must not contact alternate");
      System.out.println("PASS: healthy primary remains selected");

      URI noAlternate = URI.create(primary.toString().replace("sn-primary%2Csn-backup", "sn-primary"));
      boolean failed = false;
      try { select(noAlternate, false); } catch (Exception expected) { failed = true; }
      check(failed, "failed primary with no alternate must surface the failure");
      System.out.println("PASS: absent alternate surfaces failure");

      primaryRequests.set(0); alternateRequests.set(0);
      failed = false;
      try { select(URI.create(primary + "&allfail=1"), false); } catch (Exception expected) { failed = true; }
      check(failed, "failure on both CDNs must propagate");
      check(primaryRequests.get() == 1 && alternateRequests.get() == 1, "failed alternate must not cause repeated retries");
      System.out.println("PASS: failed alternate propagates error without looping");
    } finally { server.stop(0); }
  }

  private static URI select(URI uri, boolean refusePrimary) throws Exception {
    StreamFormat format = new StreamFormat(ContentType.parse("audio/mp4; codecs=\"mp4a.40.2\""), 140, 128000, 1000000, 2, uri.toString(), null, null, null, true, false);
    TrackFormats formats = new TrackFormats(Collections.singletonList(format), "unused");
    Client client = (Client) Proxy.newProxyInstance(Client.class.getClassLoader(), new Class<?>[] {Client.class}, (proxy, method, args) -> {
      if (method.getName().equals("supportsFormatLoading")) return true;
      if (method.getName().equals("requirePlayerScript")) return false;
      if (method.getName().equals("loadFormats")) return formats;
      if (method.getName().equals("getIdentifier")) return "TEST";
      throw new AssertionError("Unexpected client call: " + method.getName());
    });
    PoolingHttpClientConnectionManager connections = new PoolingHttpClientConnectionManager(
      RegistryBuilder.<ConnectionSocketFactory>create().register("http", PlainConnectionSocketFactory.getSocketFactory()).build(),
      host -> new InetAddress[] {InetAddress.getByName(refusePrimary && host.contains("sn-primary") ? "127.0.0.2" : "127.0.0.1")});
    HttpContextFilter filter = (HttpContextFilter) Proxy.newProxyInstance(HttpContextFilter.class.getClassLoader(), new Class<?>[] {HttpContextFilter.class},
        (proxy, method, args) -> method.getReturnType() == boolean.class ? false : null);
    try (CloseableHttpClient httpClient = HttpClients.custom().setConnectionManager(connections)
        .setDefaultRequestConfig(RequestConfig.custom().setConnectTimeout(500).setSocketTimeout(500).build()).build();
        HttpInterface http = new HttpInterface(httpClient, new HttpClientContext(), true, filter)) {
      YoutubeAudioTrack track = new YoutubeAudioTrack(new AudioTrackInfo("test", "test", 10000, "test", false, "test"), null);
      Method method = YoutubeAudioTrack.class.getDeclaredMethod("loadBestFormatWithUrl", HttpInterface.class, Client.class);
      method.setAccessible(true);
      Object result;
      try { result = method.invoke(track, http, client); }
      catch (InvocationTargetException exception) { throw (Exception) exception.getCause(); }
      Field field = result.getClass().getDeclaredField("signedUrl");
      field.setAccessible(true);
      return (URI) field.get(result);
    }
  }

  private static void check(boolean condition, String message) {
    if (!condition) throw new AssertionError(message);
  }
}
