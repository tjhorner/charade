package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

type ChannelMeta struct {
	TextChannelID *string
	MembersCount  int
}

var channelMetas map[string]*ChannelMeta
var metaMutex sync.Mutex
var channelRegex = regexp.MustCompile(`[^A-z0-9\- ]`)

func main() {
	godotenv.Load()
	channelMetas = make(map[string]*ChannelMeta)

	botToken := os.Getenv("DISCORD_BOT_TOKEN")

	discord, err := discordgo.New("Bot " + botToken)
	if err != nil {
		panic(err)
	}

	discord.AddHandler(voiceStateUpdate)
	discord.Identify.Intents = discordgo.IntentsGuildVoiceStates

	err = discord.Open()
	if err != nil {
		panic(err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	discord.Close()
}

func getMeta(state *discordgo.VoiceState) *ChannelMeta {
	metaMutex.Lock()
	defer metaMutex.Unlock()

	if val, ok := channelMetas[state.ChannelID]; ok {
		return val
	}

	channelMetas[state.ChannelID] = &ChannelMeta{}
	return channelMetas[state.ChannelID]
}

func firstN(s string, n int) string {
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}

func normalizeName(vcName string) string {
	stripped := firstN(channelRegex.ReplaceAllString(vcName, ""), 32)
	hyphenated := strings.ReplaceAll(strings.ToLower(stripped), " ", "-")
	return hyphenated
}

func userJoined(s *discordgo.Session, state *discordgo.VoiceState) {
	meta := getMeta(state)

	metaMutex.Lock()
	defer metaMutex.Unlock()

	if meta.TextChannelID == nil {
		vc, err := s.Channel(state.ChannelID)
		if err != nil {
			log.Println(err)
			return
		}

		tc, err := s.GuildChannelCreateComplex(state.GuildID, discordgo.GuildChannelCreateData{
			Name:     fmt.Sprintf("text-%s", normalizeName(vc.Name)),
			Type:     discordgo.ChannelTypeGuildText,
			ParentID: vc.ParentID,
			Topic:    fmt.Sprintf("This is an **ephemeral text channel** for the voice channel \"%s\". You can use it to send media related to conversations happening in the voice channel. It will be deleted once everyone leaves.", vc.Name),
			PermissionOverwrites: []*discordgo.PermissionOverwrite{
				{
					ID:   state.GuildID,
					Type: discordgo.PermissionOverwriteTypeRole,
					Deny: discordgo.PermissionViewChannel,
				},
				{
					ID:    s.State.User.ID,
					Type:  discordgo.PermissionOverwriteTypeMember,
					Allow: discordgo.PermissionViewChannel,
				},
			},
		})
		if err != nil {
			log.Println(err)
			return
		}

		meta.TextChannelID = &tc.ID
	}

	s.ChannelPermissionSet(*meta.TextChannelID, state.UserID, discordgo.PermissionOverwriteTypeMember, discordgo.PermissionViewChannel, 0)
	meta.MembersCount = meta.MembersCount + 1
}

func userLeft(s *discordgo.Session, state *discordgo.VoiceState) {
	meta := getMeta(state)
	if meta.TextChannelID == nil {
		return
	}

	metaMutex.Lock()
	defer metaMutex.Unlock()

	meta.MembersCount = meta.MembersCount - 1

	if meta.MembersCount <= 0 {
		s.ChannelDelete(*meta.TextChannelID)
		meta.TextChannelID = nil
	} else {
		s.ChannelPermissionSet(*meta.TextChannelID, state.UserID, discordgo.PermissionOverwriteTypeMember, 0, 0)
	}
}

func voiceStateUpdate(s *discordgo.Session, state *discordgo.VoiceStateUpdate) {
	if state.BeforeUpdate != nil && state.BeforeUpdate.ChannelID != "" && state.BeforeUpdate.ChannelID != state.ChannelID {
		userLeft(s, state.BeforeUpdate)
	}

	if state.ChannelID != "" && (state.BeforeUpdate == nil || state.BeforeUpdate.ChannelID != state.ChannelID) {
		userJoined(s, state.VoiceState)
	}
}
