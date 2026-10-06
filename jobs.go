package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) runJobs(ctx context.Context) {
	if b.cfg.ReportChannelID == "" {
		log.Println("jobs: REPORT_CHANNEL_ID vazio, jobs desativados")
		return
	}
	go schedule(ctx, func(now time.Time) time.Time { return nextWeekly(now, time.Sunday, 20) }, b.weeklyReport)
	if b.cfg.InactiveDays > 0 {
		go schedule(ctx, func(now time.Time) time.Time { return nextDaily(now, 12) }, b.inactivityCheck)
	}
}

func schedule(ctx context.Context, next func(time.Time) time.Time, fn func()) {
	for {
		t := time.NewTimer(time.Until(next(time.Now())))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			fn()
		}
	}
}

func nextDaily(now time.Time, hour int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func nextWeekly(now time.Time, day time.Weekday, hour int) time.Time {
	t := nextDaily(now, hour)
	for t.Weekday() != day {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func (b *Bot) weeklyReport() {
	b.notify("📊 Relatório semanal\n" + b.cmdRanking())
}

func (b *Bot) inactivityCheck() {
	limit := time.Duration(b.cfg.InactiveDays) * 24 * time.Hour
	now := time.Now().UTC()
	for _, e := range b.store.All() {
		if e.AlertedAt != nil || now.Sub(e.SentAt) < limit {
			continue
		}
		_, err := b.dg.ChannelMessageSendComplex(b.cfg.ReportChannelID, &discordgo.MessageSend{
			Content:         fmt.Sprintf("👀 <@%s> sumiu! Última mensagem %s.", e.UserID, ts(e.SentAt, "R")),
			AllowedMentions: &discordgo.MessageAllowedMentions{Users: []string{e.UserID}},
		})
		if err != nil {
			log.Printf("inactivity: %v", err)
			continue
		}
		if err := b.store.MarkAlerted(e.UserID, now); err != nil {
			log.Printf("inactivity: store: %v", err)
		}
	}
}
