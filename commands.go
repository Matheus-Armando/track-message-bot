package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "ultima",
		Description: "Mostra a última mensagem de alguém",
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "usuario",
			Description: "Quem",
			Required:    true,
		}},
	},
	{Name: "ranking", Description: "Quem está há mais tempo sem falar"},
	{Name: "status", Description: "Estado do bot e do armazenamento"},
}

func (b *Bot) registerCommands() error {
	_, err := b.dg.ApplicationCommandBulkOverwrite(b.dg.State.User.ID, b.cfg.GuildID, commands)
	return err
}

func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data := i.ApplicationCommandData()
	var content string
	ephemeral := false
	switch data.Name {
	case "ultima":
		content = b.cmdUltima(data.Options[0].UserValue(s))
	case "ranking":
		content = b.cmdRanking()
	case "status":
		content = b.cmdStatus()
		ephemeral = true
	default:
		return
	}

	resp := &discordgo.InteractionResponseData{
		Content:         truncate(content, 2000),
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}
	if ephemeral {
		resp.Flags = discordgo.MessageFlagsEphemeral
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: resp,
	})
	if err != nil {
		log.Printf("interaction %s: %v", data.Name, err)
	}
}

func (b *Bot) cmdUltima(u *discordgo.User) string {
	rec, ok := b.store.Get(u.ID)
	if !ok {
		return fmt.Sprintf("Nenhuma mensagem registrada de <@%s>.", u.ID)
	}
	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", b.cfg.GuildID, rec.ChannelID, rec.MessageID)
	return fmt.Sprintf("<@%s> falou por último %s (%s) em <#%s> — %s\n%s",
		u.ID, ts(rec.SentAt, "R"), ts(rec.SentAt, "f"), rec.ChannelID, link, quote(truncate(rec.Content, 1500)))
}

func (b *Bot) cmdRanking() string {
	type row struct {
		id     string
		sentAt time.Time
		known  bool
	}
	rows := map[string]row{}
	for _, e := range b.store.All() {
		rows[e.UserID] = row{id: e.UserID, sentAt: e.SentAt, known: true}
	}
	if members, err := b.dg.GuildMembers(b.cfg.GuildID, "", 1000); err == nil {
		for _, m := range members {
			if m.User.Bot {
				continue
			}
			if _, ok := rows[m.User.ID]; !ok {
				rows[m.User.ID] = row{id: m.User.ID}
			}
		}
	} else {
		log.Printf("ranking: members: %v", err)
	}
	if len(rows) == 0 {
		return "Nada registrado ainda."
	}

	list := make([]row, 0, len(rows))
	for _, r := range rows {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].known != list[j].known {
			return !list[i].known
		}
		return list[i].sentAt.Before(list[j].sentAt)
	})

	var sb strings.Builder
	sb.WriteString("**Há mais tempo sem falar:**\n")
	for n, r := range list {
		when := "nunca (desde que o bot entrou)"
		if r.known {
			when = ts(r.sentAt, "R")
		}
		fmt.Fprintf(&sb, "%d. <@%s> — %s\n", n+1, r.id, when)
	}
	return sb.String()
}

func (b *Bot) cmdStatus() string {
	meta := b.store.Meta()
	var sb strings.Builder
	fmt.Fprintf(&sb, "Usuários rastreados: %d\n", len(b.store.All()))
	fmt.Fprintf(&sb, "Último save: %s\n", ts(meta.UpdatedAt, "R"))
	if r := meta.RestoredFromBackup; r != nil {
		fmt.Fprintf(&sb, "⚠️ Restaurado do backup %s: `%s`", ts(r.At, "f"), r.Reason)
	} else {
		sb.WriteString("Nenhuma restauração de backup registrada.")
	}
	return sb.String()
}

func ts(t time.Time, style string) string {
	return fmt.Sprintf("<t:%d:%s>", t.Unix(), style)
}

func quote(s string) string {
	if s == "" {
		return ""
	}
	return "> " + strings.ReplaceAll(s, "\n", "\n> ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
