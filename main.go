package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/bwmarrin/discordgo"
	GMMAuth "github.com/maxsupermanhd/go-mc-ms-auth"
)

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "help",
			Description: "Sends help",
		},
		{
			Name:        "config",
			Description: "Configuration manipulation",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "save",
					Description: "Save config",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "load",
					Description: "Load config",
				},
			},
		},
		{
			Name:        "rooms",
			Description: "Show registered rooms",
		},
		{
			Name:        "auth",
			Description: "Manipulate authentication",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "new",
					Description: "New credentials",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "room",
							Description: "Selected room",
							Required:    false,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "check",
					Description: "View credentials status",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "refresh",
					Description: "Force renew authentication tokens",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "room",
							Description: "Selected room",
							Required:    false,
						},
					},
				},
			},
		},
		{
			Name:        "activate",
			Description: "Activate stasis",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "chamber",
					Description: "Selected stasis",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "room",
					Description: "Selected room",
					Required:    false,
				},
			},
		},
	}
	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"help":     commandHelp,
		"config":   commandConfig,
		"rooms":    commandRooms,
		"auth":     commandAuth,
		"activate": commandActivate,
	}
)

func main() {
	log.Println("Loading config...")
	err := loadConfig()
	if err != nil {
		log.Fatalf("Error reading config: %s", err.Error())
	}
	if config == nil {
		log.Fatal("No errors but no config was made")
	}
	log.Print("Verifying config...")
	err = verifyConfig()
	if err != nil {
		log.Fatalf("Error verifying config: %s", err.Error())
	}

	dg, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		log.Println("error creating Discord session,", err)
		return
	}
	// dg.AddHandler(messageCreate)
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	err = dg.Open()
	if err != nil {
		log.Println("error opening connection,", err)
		return
	}
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
	for _, v := range commands {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, config.GuildID, v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
	}
	log.Println("Bot is now running. Send SIGINT or SIGTERM to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	log.Println("Roger, stopping shit.")
	dg.Close()
	log.Println("Discord quit, exiting...")
}

func commandHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	iTextResponse(s, i, `**PearlBot usage**
/help - shows this message
/config (save|load) - config manipulation
/check - show diagnostic information
/rooms - list all registered rooms in the channel
/activate - activate pearl stasis chamber`)
}

func findRoomsByChannelID(channelID string) (ret []PearlRoom) {
	for _, r := range config.PearlRooms {
		if r.DiscordChannel == channelID {
			ret = append(ret, r)
		}
	}
	return
}

func usernameBeautify(username string) string {
	if username != "" {
		return "`" + username + "`"
	} else {
		return "(unknown)"
	}
}

func commandRooms(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rooms := findRoomsByChannelID(i.ChannelID)
	if len(rooms) <= 0 {
		iTextResponse(s, i, "No room is registered in this channel")
		return
	}
	if len(rooms) == 1 {
		username, _ := checkCredentialsValid(rooms[0].AccountCredentialsName)
		username = usernameBeautify(username)
		iTextResponse(s, i, fmt.Sprintf("Room named `%s` with `%s` as activator", rooms[0].RoomName, username))
		return
	}
	resp := fmt.Sprintf("Registered rooms in this channel: %d", len(rooms))
	for i, r := range rooms {
		username, _ := checkCredentialsValid(r.AccountCredentialsName)
		username = usernameBeautify(username)
		resp += fmt.Sprintf("\n[%d] `%s` with %d chambers and `%s` as activator (provided by <@%s>)", i, r.RoomName, len(r.Chambers), username, r.AccountOwner)
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: resp,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse: []discordgo.AllowedMentionType{},
			},
		},
	})
}

func commandActivate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rooms := findRoomsByChannelID(i.ChannelID)
	chambernum := int(i.ApplicationCommandData().Options[0].IntValue())
	var room PearlRoom
	if len(rooms) <= 0 {
		iTextResponse(s, i, "Channel does not have any rooms attached")
		return
	}
	if len(rooms) == 1 {
		room = rooms[0]
	} else {
		if len(i.ApplicationCommandData().Options) < 2 {
			iTextResponse(s, i, "Channel have more than one room attached, please specify room name")
			return
		}
		roomname := i.ApplicationCommandData().Options[1].StringValue()
		roomfound := false
		for _, r := range rooms {
			if r.RoomName == roomname {
				room = r
				roomfound = true
			}
		}
		if !roomfound {
			iTextResponse(s, i, "Room `"+roomname+"` not found")
			return
		}
	}
	chamberfound := false
	for _, c := range room.Chambers {
		if c.Index == chambernum {
			chamberfound = true
			break
		}
	}
	if !chamberfound {
		iTextResponse(s, i, fmt.Sprintf("Chamber %d in room %s not found", chambernum, room.RoomName))
		return
	}
	cache, err := getCredentialsCache(room.AccountCredentialsName)
	if err != nil {
		iTextResponse(s, i, "Failed to load credentials: "+err.Error())
		return
	}
	if isDateExpired(cache.Minecraft.ExpiresAfter) {
		iTextResponse(s, i, "Minecraft token expired, refreshing everything...")
		err := GMMAuth.CheckRefreshMS(&cache.Microsoft, config.MicrosoftCID)
		if err != nil {
			iTextResponse(s, i, "Failed to refresh Microsoft credentials: "+err.Error())
			return
		}
		XBLt, err := GMMAuth.AuthXBL(cache.Microsoft.AccessToken)
		if err != nil {
			iTextResponse(s, i, "Failed to refresh credentials, unable to get XBL token: "+err.Error())
			return
		}
		XSTSt, err := GMMAuth.AuthXSTS(XBLt)
		if err != nil {
			iTextResponse(s, i, "Failed to refresh credentials, unable to get XSTS token: "+err.Error())
			return
		}
		cache.Minecraft, err = GMMAuth.AuthMC(XSTSt)
		if err != nil {
			iTextResponse(s, i, "Failed to refresh credentials, unable to get MC token: "+err.Error())
			return
		}
		profile, err := GMMAuth.GetMCprofile(cache.Minecraft.Token)
		if err != nil {
			iTextResponse(s, i, "Unable to get MC profile: "+err.Error())
			return
		}
		cache.Username = profile.Name
		cache.UUID = profile.UUID
		err = writeCredentialsCache(room.AccountCredentialsName, cache)
		if err != nil {
			iTextResponse(s, i, "Unable to write credentials cache: "+err.Error())
			return
		}
	}
	triggerChamber(s, room, chambernum, bot.Auth{Name: cache.Username, UUID: cache.UUID, AsTk: cache.Minecraft.Token})
}

func triggerChamber(s *discordgo.Session, room PearlRoom, cid int, auth bot.Auth) {
	mcClient := bot.NewClient()
	mcClient.Auth = auth
	_ = basic.NewPlayer(mcClient, basic.Settings{Locale: "en_US"})
	basic.EventsListener{
		GameStart: nil,
		ChatMsg:   nil,
		Disconnect: func(c chat.Message) error {
			s.ChannelMessageSend(room.DiscordChannel, "I got disconnected for this reason: "+c.ClearString())
			return nil
		},
		Death: func() error {
			s.ChannelMessageSend(room.DiscordChannel, "Yo wtf I died!")
			return nil
		},
	}.Attach(mcClient)
	err := mcClient.JoinServer(room.ServerAdress)
	if err != nil {
		s.ChannelMessageSend(room.DiscordChannel, "Error auth: "+err.Error())
		return
	}
	s.ChannelMessageSend(room.DiscordChannel, "Logged in")
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
		pk.VarInt(0), //hand
		pk.Position(pk.Position{X: room.Chambers[cid].X, Y: room.Chambers[cid].Y, Z: room.Chambers[cid].Z}), //((int64(x)&0x3FFFFFF)<<38)|((int64(z)&0x3FFFFFF)<<12)|(int64(y)&0xFFF)), //position
		pk.VarInt(4),      //direction
		pk.Float(0.836),   //cursor x
		pk.Float(0.187),   //y
		pk.Float(0.5),     //z
		pk.Boolean(false), //inside
	))
	mcClient.Conn.WritePacket(pk.Marshal(
		packetid.ServerboundSwing,
		pk.VarInt(0), //hand
	))
	s.ChannelMessageSend(room.DiscordChannel, "Activated.")
	time.Sleep(400 * time.Millisecond)
	mcClient.Close()
}
