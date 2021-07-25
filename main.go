package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// Chamber from file
type Chamber struct {
	Index int
	X     int
	Y     int
	Z     int
}

func main() {
	log.Println("Loading enviroment")
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file: " + err.Error())
		return
	}
	Token := os.Getenv("TOKEN")
	dg, err := discordgo.New("Bot " + Token)
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
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	log.Println("Roger, stopping shit.")
	dg.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
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

	if cmd == "activate" || cmd == "trigger" {
		s.MessageReactionAdd(m.ChannelID, m.ID, "⏳")
		return
	} else if cmd == "show" {
		msg := fmt.Sprintf("Chamber %d registered at X%d Y%d Z%d", selected.Index, selected.X, selected.Y, selected.Z)
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}
}
