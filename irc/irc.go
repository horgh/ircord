package irc

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/horgh/irc"
	"github.com/pkg/errors"
)

// New creates a new Client.
func New(nick, host string, port int, tls bool) *Client {
	return &Client{
		host:        host,
		messageChan: make(chan irc.Message, 100), // 100 is arbitrary.
		nick:        nick,
		port:        port,
		tls:         tls,
	}
}

// Client is an IRC client.
type Client struct {
	channels    []string
	handlers    []func(irc.Message)
	host        string
	messageChan chan irc.Message
	nick        string
	port        int
	tls         bool
}

// Start starts the Client.
func (c *Client) Start() {
	go c.run()
}

func (c *Client) run() {
	var conn *conn
	for {
		if conn == nil {
			conn2, err := newConn(c)
			if err != nil {
				log.Printf("error connecting: %s", err)
				time.Sleep(10 * time.Second)
				continue
			}
			conn = conn2
		}

		select {
		case m := <-c.messageChan:
			conn.writeChan <- m
			if m.Command == "QUIT" {
				return
			}
		case m, ok := <-conn.readChan:
			if !ok {
				close(conn.writeChan)
				_ = conn.conn.Close()
				conn = nil
				break
			}
			if m.Command == "PING" {
				conn.writeChan <- irc.Message{
					Command: "PONG",
					Params:  []string{m.Params[0]},
				}
			}
			for _, f := range c.handlers {
				f(m)
			}
		}
	}
}

type conn struct {
	conn      net.Conn
	readChan  chan irc.Message
	rw        *bufio.ReadWriter
	writeChan chan irc.Message
}

func newConn(c *Client) (*conn, error) {
	conn := &conn{
		readChan:  make(chan irc.Message, 100), // 100 is arbitrary
		writeChan: make(chan irc.Message, 100), // 100 is arbitrary
	}
	if c.tls {
		conn2, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", c.host, c.port), nil)
		if err != nil {
			return nil, errors.Wrap(err, "error connecting")
		}
		conn.conn = conn2
	} else {
		conn2, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.host, c.port))
		if err != nil {
			return nil, errors.Wrap(err, "error connecting")
		}
		conn.conn = conn2
	}
	conn.rw = bufio.NewReadWriter(
		bufio.NewReader(conn.conn),
		bufio.NewWriter(conn.conn),
	)

	go conn.reader()
	go conn.writer()

	conn.writeChan <- irc.Message{
		Command: "NICK",
		Params:  []string{c.nick},
	}
	conn.writeChan <- irc.Message{
		Command: "USER",
		Params:  []string{c.nick, "0", "*", c.nick},
	}
	for _, channel := range c.channels {
		conn.writeChan <- irc.Message{
			Command: "JOIN",
			Params:  []string{channel},
		}
	}

	return conn, nil
}

func (c *conn) reader() {
	for {
		line, err := c.rw.ReadString('\n')
		if err != nil {
			log.Printf("%s", errors.Wrap(err, "error reading"))
			close(c.readChan)
			return
		}

		m, err := irc.ParseMessage(line)
		if err != nil {
			log.Printf("%s", errors.Wrap(err, "error parsing message"))
			continue
		}

		c.readChan <- m
	}
}

func (c *conn) writer() {
	for {
		m, ok := <-c.writeChan
		if !ok {
			return
		}

		buf, err := m.Encode()
		if err != nil {
			log.Printf("%s", errors.Wrap(err, "error encoding message"))
			continue
		}

		if _, err := c.rw.WriteString(buf); err != nil {
			log.Printf("%s", errors.Wrap(err, "error writing"))
			return
		}

		if err := c.rw.Flush(); err != nil {
			log.Printf("%s", errors.Wrap(err, "error flushing"))
			return
		}
	}
}

// Join tells the Client to join the channel.
func (c *Client) Join(channel string) {
	c.messageChan <- irc.Message{
		Command: "JOIN",
		Params:  []string{channel},
	}
	c.channels = append(c.channels, channel)
}

// Message sends a message to the target.
func (c *Client) Message(to, message string) {
	c.messageChan <- irc.Message{
		Command: "PRIVMSG",
		Params:  []string{to, message},
	}
}

// AddHandler registers a function to be called on every message the Client
// sees.
func (c *Client) AddHandler(f func(irc.Message)) {
	c.handlers = append(c.handlers, f)
}

// Close cleans up the Client.
func (c *Client) Close() error {
	c.messageChan <- irc.Message{
		Command: "QUIT",
		Params:  []string{"Bye"},
	}

	time.Sleep(time.Second) // Let writer send quit

	return nil
}
