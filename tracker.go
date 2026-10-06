package main

import (
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

const backfillMaxPages = 20

func (b *Bot) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID != b.cfg.GuildID {
		return
	}
	e := entryFrom(m.Message)
	if m.Member != nil && m.Member.Nick != "" {
		e.Username = m.Member.Nick
	}
	if err := b.store.Upsert(e); err != nil {
		log.Printf("store: %v", err)
	}
}

func entryFrom(m *discordgo.Message) Entry {
	content := m.Content
	if content == "" {
		switch {
		case len(m.Attachments) > 0:
			content = "[anexo]"
		case len(m.StickerItems) > 0:
			content = "[sticker]"
		case len(m.Embeds) > 0:
			content = "[embed]"
		}
	}
	return Entry{
		UserID: m.Author.ID,
		UserRecord: UserRecord{
			Username:  m.Author.DisplayName(),
			ChannelID: m.ChannelID,
			MessageID: m.ID,
			Content:   content,
			SentAt:    m.Timestamp.UTC(),
		},
	}
}

// backfill scans recent channel history to catch messages sent while the bot was offline.
func (b *Bot) backfill() {
	var cutoff time.Time
	if !b.restored && !b.store.Empty() {
		cutoff = b.store.Meta().UpdatedAt.Add(-time.Hour)
	}

	channels, err := b.dg.GuildChannels(b.cfg.GuildID)
	if err != nil {
		log.Printf("backfill: channels: %v", err)
		return
	}
	if active, err := b.dg.GuildThreadsActive(b.cfg.GuildID); err == nil {
		channels = append(channels, active.Threads...)
	}

	var found []Entry
	for _, ch := range channels {
		switch ch.Type {
		case discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews,
			discordgo.ChannelTypeGuildPublicThread, discordgo.ChannelTypeGuildPrivateThread,
			discordgo.ChannelTypeGuildNewsThread, discordgo.ChannelTypeGuildVoice:
		default:
			continue
		}
		found = append(found, b.scanChannel(ch.ID, cutoff)...)
	}

	if err := b.store.Upsert(found...); err != nil {
		log.Printf("backfill: store: %v", err)
	}
	log.Printf("backfill: %d canais, %d mensagens relevantes", len(channels), len(found))
}

func (b *Bot) scanChannel(channelID string, cutoff time.Time) []Entry {
	seen := map[string]bool{}
	var out []Entry
	before := ""
	for page := 0; page < backfillMaxPages; page++ {
		msgs, err := b.dg.ChannelMessages(channelID, 100, before, "", "")
		if err != nil || len(msgs) == 0 {
			return out
		}
		for _, m := range msgs {
			if m.Timestamp.Before(cutoff) {
				return out
			}
			if m.Author == nil || m.Author.Bot || seen[m.Author.ID] {
				continue
			}
			seen[m.Author.ID] = true
			out = append(out, entryFrom(m))
		}
		before = msgs[len(msgs)-1].ID
	}
	return out
}
