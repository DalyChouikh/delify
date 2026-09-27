package bot

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestVoiceEventsBeforeInitialization(t *testing.T) {
	s, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatal(err)
	}
	s.State.User = &discordgo.User{ID: "1"}
	b := &Bot{session: s}
	b.handleVoiceStateUpdate(s, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: "2", UserID: "1", ChannelID: "3"}})
	b.handleVoiceServerUpdate(s, &discordgo.VoiceServerUpdate{GuildID: "2", Endpoint: "voice.example"})
}
