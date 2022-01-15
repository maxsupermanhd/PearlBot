package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	GMMAuth "github.com/maxsupermanhd/go-mc-ms-auth"
)

// Chamber from file
type Chamber struct {
	Index int
	X     int
	Y     int
	Z     int
}

// DiscordToken token
var DiscordToken string

func main() {
	log.Println("Loading enviroment")
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file: " + err.Error())
		return
	}
	DiscordToken = os.Getenv("TOKEN")

	dg, err := discordgo.New("Bot " + DiscordToken)
	if err != nil {
		log.Println("error creating Discord session,", err)
		return
	}
	dg.AddHandler(messageCreate)
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	err = dg.Open()
	if err != nil {
		log.Println("error opening connection,", err)
		return
	}
	log.Println("Bot is now running. Send SIGINT or SIGTERM to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	log.Println("Roger, stopping shit.")
	dg.Close()
}

var x, y, z int

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	if len(m.Content) < 2 {
		return
	}
	if m.Content[0] != '!' {
		return
	}
	var cmd string
	var param int
	n, _ := fmt.Sscanf(m.Content, "!%s %d", &cmd, &param)
	// if err != nil {
	// 	s.ChannelMessageSend(m.ChannelID, "Error parsing command: "+err.Error())
	// 	return
	// }
	if cmd == "fuckyou" {
		s.ChannelMessageSend(m.ChannelID, "No fuck YOU!")
		return
	}

	f, err := os.Open("chambers.lol")
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error opening chambers file: "+err.Error())
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var chambers []Chamber
	for scanner.Scan() {
		var chambernum, posx, posy, posz int
		nn, err := fmt.Sscanf(scanner.Text(), "%d %d %d %d", &chambernum, &posx, &posy, &posz)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error parsing chambers content: "+err.Error())
			return
		}
		if nn != 4 {
			s.ChannelMessageSend(m.ChannelID, "Strange shit with chambers content: "+err.Error())
			return
		}
		chambers = append(chambers, Chamber{chambernum, posx, posy, posz})
	}
	if err := scanner.Err(); err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error reading chambers file: "+err.Error())
		return
	}

	if len(chambers) == 0 {
		s.ChannelMessageSend(m.ChannelID, "Sorry, no chambers registered")
		return
	}
	if n < 2 {
		s.ChannelMessageSend(m.ChannelID, "`!"+cmd+"` require one argument")
		return
	}
	if param < 0 {
		s.ChannelMessageSend(m.ChannelID, "Sorry, nether does not have any stasis chambers")
		return
	}
	if param >= len(chambers) {
		s.ChannelMessageSend(m.ChannelID, "Sorry, we have only "+strconv.Itoa(len(chambers))+" chambers")
		return
	}
	var selected Chamber
	selected.Index = -1
	for i, j := range chambers {
		if j.Index == param {
			selected = chambers[i]
			break
		}
	}
	if selected.Index == -1 {
		s.ChannelMessageSend(m.ChannelID, "Chamber "+strconv.Itoa(len(chambers))+" not found")
		return
	}
	x = selected.X
	y = selected.Y
	z = selected.Z

	if cmd == "activate" || cmd == "trigger" {
		s.MessageReactionAdd(m.ChannelID, m.ID, "⏳")
		mauth, err := GMMAuth.GetMCcredentials("", "")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error auth: "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, "Authenticated as `"+mauth.Name+"` (`"+mauth.UUID+"`)")
		mcClient := bot.NewClient()
		mcClient.Auth = mauth
		_ = basic.NewPlayer(mcClient, basic.Settings{Locale: "en_US"})
		basic.EventsListener{
			GameStart: nil,
			ChatMsg:   nil,
			Disconnect: func(c chat.Message) error {
				s.ChannelMessageSend(m.ChannelID, "I got disconnected for this reason: "+c.ClearString())
				return nil
			},
			Death: func() error {
				s.ChannelMessageSend(m.ChannelID, "Yo wtf I died!")
				return nil
			},
		}.Attach(mcClient)
		err = mcClient.JoinServer("simplyvanilla.net")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error auth: "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, "Logged in")
		go mcClient.HandleGame()

		time.Sleep(1 * time.Second)
		// mcClient.Conn.WritePacket(pk.Marshal(
		// 	0x14,
		// 	pk.Float(-106.0), //cursor x
		// 	pk.Float(34.0),   //y
		// 	pk.Boolean(true), //on ground
		// ))
		mcClient.Conn.WritePacket(pk.Marshal(
			packetid.ServerboundUseItemOn,
			pk.VarInt(0),                               //hand
			pk.Position(pk.Position{X: x, Y: y, Z: z}), //((int64(x)&0x3FFFFFF)<<38)|((int64(z)&0x3FFFFFF)<<12)|(int64(y)&0xFFF)), //position
			pk.VarInt(4),                               //direction
			pk.Float(0.836),                            //cursor x
			pk.Float(0.187),                            //y
			pk.Float(0.5),                              //z
			pk.Boolean(false),                          //inside
		))
		mcClient.Conn.WritePacket(pk.Marshal(
			packetid.ServerboundSwing,
			pk.VarInt(0), //hand
		))
		s.ChannelMessageSend(m.ChannelID, "Activated.")
		time.Sleep(400 * time.Millisecond)
		mcClient.Close()
		return
	} else if cmd == "show" {
		msg := fmt.Sprintf("Chamber %d registered at X%d Y%d Z%d", selected.Index, selected.X, selected.Y, selected.Z)
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}
}
