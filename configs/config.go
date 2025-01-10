package configs

type Config struct {
    DiscordToken string
}

func NewConfig(token string) *Config {
    return &Config{
        DiscordToken: token,
    }
}
