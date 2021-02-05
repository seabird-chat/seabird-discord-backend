package main

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"

	seabird_discord "github.com/seabird-chat/seabird-discord-backend"
)

func EnvDefault(key string, def string) string {
	if ret, ok := os.LookupEnv(key); ok {
		return ret
	}
	return def
}

func Env(logger zerolog.Logger, key string) string {
	ret, ok := os.LookupEnv(key)

	if !ok {
		logger.Fatal().Str("var", key).Msg("Required environment variable not found")
	}

	return ret
}

func main() {
	// Attempt to load from .env if it exists
	_ = godotenv.Load()

	var logger zerolog.Logger

	if isatty.IsTerminal(os.Stdout.Fd()) {
		logger = zerolog.New(zerolog.NewConsoleWriter())
	} else {
		logger = zerolog.New(os.Stdout)
	}

	logger = logger.With().Timestamp().Logger()
	logger.Level(zerolog.InfoLevel)

	config := seabird_discord.DiscordConfig{
		DiscordToken:          Env(logger, "DISCORD_TOKEN"),
		CommandPrefix:         EnvDefault("DISCORD_COMMAND_PREFIX", "!"),
		SeabirdID:             EnvDefault("SEABIRD_ID", "seabird"),
		SeabirdHost:           Env(logger, "SEABIRD_HOST"),
		SeabirdToken:          Env(logger, "SEABIRD_TOKEN"),
		DiscordChannelMapping: EnvDefault("DISCORD_CHANNEL_MAP", ""),
		Logger:                logger,
	}

	backend, err := seabird_discord.New(config)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load backend")
	}

	err = backend.Run()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to run backend")
	}
}
