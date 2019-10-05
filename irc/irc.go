package irc

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"

	"github.com/horgh/irc"
	"github.com/pkg/errors"
)

// New creates a new Client.
func New(nick, host string, port int, tls bool) *Client {
	return &Client{
		eventChan: make(chan event, 100), // 100 is arbitrary.
		nick:      nick,
		host:      host,
		port:      port,
		tls:       tls,
		writeChan: make(chan irc.Message, 100), // 100 is arbitrary.
	}
}

// Client is an IRC client.
type Client struct {
	conn       net.Conn
	handlers   []func(irc.Message)
	eventChan  chan event
	rw         *bufio.ReadWriter
	nick       string
	host       string
	port       int
	tls        bool
	registered bool
	writeChan  chan irc.Message
}

type event struct {
	args    []string
	kind    kind
	message irc.Message
}

type kind int

const (
	unknown kind = iota
	readMessage
	joinCommand
	messageCommand
)

// Start starts the Client.
func (c *Client) Start() error {
	if err := c.connect(); err != nil {
		return err
	}

	c.writeChan <- irc.Message{
		Command: "NICK",
		Params:  []string{c.nick},
	}
	c.writeChan <- irc.Message{
		Command: "USER",
		Params:  []string{c.nick, "0", "*", c.nick},
	}

	go c.run()
	return nil
}

func (c *Client) connect() error {
	if c.tls {
		conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", c.host, c.port), nil)
		if err != nil {
			return errors.Wrap(err, "error connecting")
		}
		c.conn = conn
	} else {
		conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.host, c.port))
		if err != nil {
			return errors.Wrap(err, "error connecting")
		}
		c.conn = conn
	}
	c.rw = bufio.NewReadWriter(bufio.NewReader(c.conn), bufio.NewWriter(c.conn))
	go c.reader()
	go c.writer()
	return nil
}

func (c *Client) run() {
	for {
		e := <-c.eventChan

		if e.kind == readMessage {
			if e.message.Command == "PING" {
				c.writeChan <- irc.Message{
					Command: "PONG",
					Params:  []string{e.message.Params[0]},
				}
				continue
			}

			for _, f := range c.handlers {
				f(e.message)
			}
			continue
		}

		if e.kind == joinCommand {
			c.writeChan <- irc.Message{
				Command: "JOIN",
				Params:  []string{e.args[0]},
			}
			continue
		}

		if e.kind == messageCommand {
			c.writeChan <- irc.Message{
				Command: "PRIVMSG",
				Params:  e.args,
			}
			continue
		}
	}
}

func (c *Client) reader() {
	for {
		line, err := c.rw.ReadString('\n')
		if err != nil {
			log.Printf("%s", errors.Wrap(err, "error reading"))
			// XXX send event so we reconnect
			return
		}

		m, err := irc.ParseMessage(line)
		if err != nil {
			log.Printf("%s", errors.Wrap(err, "error parsing message"))
			continue
		}

		c.eventChan <- event{kind: readMessage, message: m}
	}
}

func (c *Client) writer() {
	for {
		m := <-c.writeChan

		buf, err := m.Encode()
		if err != nil {
			log.Printf("%s", errors.Wrap(err, "error encoding message"))
			continue
		}

		if _, err := c.rw.WriteString(buf); err != nil {
			log.Printf("%s", errors.Wrap(err, "error writing"))
			// XXX send event so we reconnect
			return
		}

		if err := c.rw.Flush(); err != nil {
			log.Printf("%s", errors.Wrap(err, "error flushing"))
			// XXX send event so we reconnect
			return
		}
	}
}

// Join tells the Client to join the channel.
func (c *Client) Join(channel string) {
	c.eventChan <- event{kind: joinCommand, args: []string{channel}}
}

// Message sends a message to the target.
func (c *Client) Message(to, message string) {
	c.eventChan <- event{kind: messageCommand, args: []string{to, message}}
}

// AddHandler registers a function to be called on every message the Client
// sees.
func (c *Client) AddHandler(f func(irc.Message)) {
	c.handlers = append(c.handlers, f)
}

// Close cleans up the Client.
func (c *Client) Close() error {
	c.registered = false
	c.rw = nil

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.conn = nil
			return errors.Wrap(err, "error closing connection")
		}
		c.conn = nil
	}

	return nil
}
