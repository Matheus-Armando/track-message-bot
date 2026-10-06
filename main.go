package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	_ "time/tzdata"

	"github.com/bwmarrin/discordgo"
)

type Config struct {
	Token           string
	GuildID         string
	DataDir         string
	ReportChannelID string
	InactiveDays    int
}

type Bot struct {
	cfg      Config
	dg       *discordgo.Session
	store    *Store
	restored bool
}

func loadConfig() Config {
	cfg := Config{
		Token:           getenv("DISCORD_TOKEN"),
		GuildID:         getenv("GUILD_ID"),
		DataDir:         envOr("DATA_DIR", "/data"),
		ReportChannelID: getenv("REPORT_CHANNEL_ID"),
		InactiveDays:    7,
	}
	if v := getenv("INACTIVE_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("INACTIVE_DAYS inválido: %v", err)
		}
		cfg.InactiveDays = n
	}
	if cfg.Token == "" || cfg.GuildID == "" {
		log.Fatal("DISCORD_TOKEN e GUILD_ID são obrigatórios")
	}
	return cfg
}

func envOr(key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	cfg := loadConfig()

	store, restored, err := OpenStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	dg, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuildMembers

	b := &Bot{cfg: cfg, dg: dg, store: store, restored: restored}
	dg.AddHandlerOnce(b.onReady)
	dg.AddHandler(b.onMessage)
	dg.AddHandler(b.onInteraction)

	if err := dg.Open(); err != nil {
		log.Fatalf("discord open: %v", err)
	}
	defer dg.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go b.runJobs(ctx)

	<-ctx.Done()
	log.Println("shutting down")
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("online como %s", r.User.Username)
	found := false
	for _, g := range r.Guilds {
		log.Printf("servidor: %s", g.ID)
		found = found || g.ID == b.cfg.GuildID
	}
	if !found {
		log.Printf("AVISO: o bot não está no GUILD_ID %s", b.cfg.GuildID)
	}
	if err := b.registerCommands(); err != nil {
		log.Printf("commands: %v", err)
	}
	if b.restored {
		meta := b.store.Meta()
		b.notify("⚠️ O estado foi restaurado a partir do backup: `" + meta.RestoredFromBackup.Reason + "`")
	}
	go b.backfill()
}

func (b *Bot) notify(content string) {
	if b.cfg.ReportChannelID == "" {
		return
	}
	_, err := b.dg.ChannelMessageSendComplex(b.cfg.ReportChannelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	})
	if err != nil {
		log.Printf("notify: %v", err)
	}
}

// getenv strips inline "# comment" suffixes, which plain .env loaders keep in the value.
func getenv(key string) string {
	v, _, _ := strings.Cut(os.Getenv(key), " #")
	return strings.TrimSpace(v)
}
