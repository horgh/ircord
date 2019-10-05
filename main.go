package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	irclib "github.com/horgh/irc"
	"github.com/horgh/ircord/irc"
	"github.com/pkg/errors"
)

func main() {
	nick := flag.String("nick", "", "IRC nick")
	host := flag.String("host", "", "IRC server host")
	port := flag.Int("port", 7000, "IRC server port")
	tls := flag.Bool("tls", true, "IRC server uses TLS?")
	ircChannel := flag.String("irc-channel", "", "IRC channel to relay")
	discordChannelStr := flag.String(
		"discord-channel",
		"",
		"Discord channel to relay",
	)

	flag.Parse()

	if *nick == "" || *host == "" || *ircChannel == "" ||
		*discordChannelStr == "" {
		flag.Usage()
		os.Exit(1)
	}

	token := os.Getenv("IRCORD_TOKEN")
	if token == "" {
		log.Fatalf("%+v", errors.New("you must set a token"))
	}

	s, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("%+v", errors.Wrap(err, "error creating client"))
	}

	discordChannel, err := getDiscordChannel(s, *discordChannelStr)
	if err != nil {
		log.Fatalf("%+v", err)
	}

	ircClient := irc.New(*nick, *host, *port, *tls)

	ircord := ircord{
		channels:       map[string]*discordgo.Channel{},
		discordChannel: discordChannel,
		ircChannel:     *ircChannel,
		ircClient:      ircClient,
		session:        s,
		users:          map[string]*discordgo.User{},
	}

	ircClient.AddHandler(ircord.ircMessage)

	if err := ircClient.Start(); err != nil {
		log.Fatalf("%+v", err)
	}

	ircClient.Join(*ircChannel)

	s.AddHandler(ircord.messageCreate)

	if err := s.Open(); err != nil {
		log.Fatalf("%+v", errors.Wrap(err, "error connecting"))
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-ch

	if err := s.Close(); err != nil {
		log.Printf("%+v", errors.Wrap(err, "error closing session"))
	}

	if err := ircClient.Close(); err != nil {
		log.Printf("%+v", err)
	}
}

func getDiscordChannel(
	s *discordgo.Session,
	name string,
) (*discordgo.Channel, error) {
	limit := 100
	var beforeID, afterID string
	guilds, err := s.UserGuilds(limit, beforeID, afterID)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving guilds")
	}

	for _, g := range guilds {
		channels, err := s.GuildChannels(g.ID)
		if err != nil {
			return nil, errors.Wrap(err, "error retrieving channels")
		}

		for _, c := range channels {
			if c.Name == name {
				return c, nil
			}
		}
	}

	return nil, errors.New("channel not found")
}

type ircord struct {
	channels       map[string]*discordgo.Channel
	discordChannel *discordgo.Channel
	ircChannel     string
	ircClient      *irc.Client
	session        *discordgo.Session
	users          map[string]*discordgo.User
}

func (i *ircord) ircMessage(m irclib.Message) {
	if m.Command != "PRIVMSG" {
		return
	}
	if m.Params[0] != i.ircChannel { // TODO(horgh): Case?
		return
	}

	if _, err := i.session.ChannelMessageSend(
		i.discordChannel.ID,
		fmt.Sprintf("<%s> %s", m.SourceNick(), m.Params[1]),
	); err != nil {
		log.Printf("%+v", errors.Wrap(err, "error sending message"))
	}
}

func (i *ircord) messageCreate(
	s *discordgo.Session,
	m *discordgo.MessageCreate,
) {
	if m.Author.ID == s.State.User.ID { // From ourself.
		return
	}

	if m.ChannelID != i.discordChannel.ID {
		return
	}

	if m.Content != "" {
		re := regexp.MustCompile(`<@!?[0-9]+>`)
		content := re.ReplaceAllStringFunc(
			m.Content,
			func(ss string) string {
				id := strings.Trim(ss, "<@!>")
				user, ok := i.users[id]
				if !ok {
					user2, err := s.User(id)
					if err != nil {
						log.Printf("%+v", errors.Wrap(err, "error retrieving user"))
						return ss
					}
					i.users[id] = user2
					user = user2
				}
				return "@" + user.Username
			},
		)

		i.ircClient.Message(
			i.ircChannel,
			fmt.Sprintf("<%s> %s", m.Author.Username, content),
		)
	}

	for _, a := range m.Attachments {
		if a.URL == "" {
			continue
		}
		i.ircClient.Message(
			i.ircChannel,
			fmt.Sprintf("<%s> %s", m.Author.Username, a.URL),
		)
	}
}
