package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

func main() {
	token := os.Getenv("IRCORD_TOKEN")
	if token == "" {
		log.Fatalf("%+v", errors.New("you must set a token"))
	}

	d, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("%+v", errors.Wrap(err, "error creating client"))
	}

	d.AddHandler(messageCreate)

	if err := d.Open(); err != nil {
		log.Fatalf("%+v", errors.Wrap(err, "error connecting"))
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-ch

	d.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID { // From ourself.
		return
	}

	log.Printf("read %s", m.Content)
}
