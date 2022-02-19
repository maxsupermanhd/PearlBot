package main

import (
	"fmt"
	"log"
	"math"
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
					Type:        discordgo.ApplicationCommandOptionInteger,
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
		// {
		// 	Name:        "status",
		// 	Description: "Spew out facts",
		// },
		// {
		// 	Name:        "bots",
		// 	Description: "Check out bots that are online",
		// },
	}
	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"help":     commandHelp,
		"config":   commandConfig,
		"rooms":    commandRooms,
		"auth":     commandAuth,
		"activate": commandActivate,
		// "status":   commandStatus,
		// "bots":     commandBots,
	}
	// botsOnline = []bot.Client{}
	dangerousActivations = map[string]activationRequest{}
)

type activationRequest struct {
	when     time.Time
	roomname string
	byUser   string
}

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
	go func() {
		for k, v := range dangerousActivations {
			if time.Until(v.when) < -120*time.Second {
				delete(dangerousActivations, k)
			}
		}
		time.Sleep(120 * time.Second)
	}()
	log.Print("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + config.DiscordToken)
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
	defer dg.Close()
	log.Print("Connected to Discord.")
	if config.ReInitCommands {
		log.Print("Removing old commands...")
		cmds, err := dg.ApplicationCommands(config.ApplicationID, "")
		if err != nil {
			log.Fatalf("Failed to get app commands: %s", err.Error())
		}
		for _, j := range cmds {
			dg.ApplicationCommandDelete(j.ApplicationID, "", j.ID)
		}
		cmds, err = dg.ApplicationCommands(config.ApplicationID, config.GuildID)
		if err != nil {
			log.Fatalf("Failed to get app commands: %s", err.Error())
		}
		for _, j := range cmds {
			err := dg.ApplicationCommandDelete(j.ApplicationID, config.GuildID, j.ID)
			if err != nil {
				log.Printf("Failed to delete old command %s", j.ID)
			}
		}
		log.Print("Registering new commands...")
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
	} else {
		dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		})
	}
	log.Println("Bot is now running. Send SIGINT or SIGTERM to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	log.Println("Roger, stopping shit.")
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
	resp := fmt.Sprintf("Registered rooms in this channel: %d\n", len(rooms))
	for _, r := range rooms {
		username, _ := checkCredentialsValid(r.AccountCredentialsName)
		username = usernameBeautify(username)
		resp += fmt.Sprintf("`%s` with %d chambers and `%s` as activator (provided by <@%s>)\n", r.RoomName, len(r.Chambers), username, r.AccountOwner)
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

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Content != "Yes I am sure, do as I say!" {
		return
	}
	if t, ok := dangerousActivations[m.ChannelID]; ok {
		if t.byUser == m.Author.ID {
			if time.Until(t.when) < -15*time.Second {
				s.ChannelMessageSend(m.ChannelID, "You did not confirmed activation fast enough.")
				delete(dangerousActivations, m.ChannelID)
				return
			} else {
				rooms := findRoomsByChannelID(m.ChannelID)
				roomfound := false
				var room PearlRoom
				for _, r := range rooms {
					if r.RoomName == t.roomname {
						room = r
						roomfound = true
					}
				}
				if !roomfound {
					s.ChannelMessageSend(m.ChannelID, "Room not found?!")
					delete(dangerousActivations, m.ChannelID)
					return
				}
				activateRoom(s, room, -1)
			}
		} else {
			return
		}
	}
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
	chamberindex := 0
	if chambernum == -1 {
		chamberfound = true
		chamberindex = -1
		if t, ok := dangerousActivations[i.ChannelID]; ok {
			if t.byUser == i.Member.User.ID {
				iTextResponse(s, i, "Activation confirmation awaiting")
			} else {
				iTextResponse(s, i, "Other member already requested activation of everything, wait until he confirms it.")
			}
			return
		} else {
			dangerousActivations[i.ChannelID] = activationRequest{
				when:     time.Now(),
				byUser:   i.Member.User.ID,
				roomname: room.RoomName,
			}
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Activation double check requested",
					Embeds: []*discordgo.MessageEmbed{
						{
							Title:       "Warning!",
							Description: "This action will activate **all** chambers in the room!\nRespond with `Yes I am sure, do as I say!` in this channel within 15 seconds to confirm",
							Color:       0xef2929,
						},
					},
				},
			})
			if err != nil {
				log.Print(err)
			}
			return
		}
	} else {
		for index, c := range room.Chambers {
			if c.Index == chambernum {
				chamberfound = true
				chamberindex = index
				break
			}
		}
		if !chamberfound {
			iTextResponse(s, i, fmt.Sprintf("Chamber %d in room %s not found", chambernum, room.RoomName))
			return
		}
	}
	activateRoom(s, room, chamberindex)
}

func activateRoom(s *discordgo.Session, room PearlRoom, cid int) {
	s.ChannelMessageSend(room.DiscordChannel, fmt.Sprintf("Activating chamber %d in room %s...", cid, room.RoomName))
	cache, err := getCredentialsCache(room.AccountCredentialsName)
	if err != nil {
		s.ChannelMessageSend(room.DiscordChannel, "Failed to load credentials: "+err.Error())
		return
	}
	if isDateExpired(cache.Minecraft.ExpiresAfter) {
		s.ChannelMessageSend(room.DiscordChannel, "Minecraft token expired, refreshing everything...")
		err := GMMAuth.CheckRefreshMS(&cache.Microsoft, config.MicrosoftCID)
		if err != nil {
			s.ChannelMessageSend(room.DiscordChannel, "Failed to refresh Microsoft credentials: "+err.Error())
			return
		}
		XBLt, err := GMMAuth.AuthXBL(cache.Microsoft.AccessToken)
		if err != nil {
			s.ChannelMessageSend(room.DiscordChannel, "Failed to refresh credentials, unable to get XBL token: "+err.Error())
			return
		}
		XSTSt, err := GMMAuth.AuthXSTS(XBLt)
		if err != nil {
			s.ChannelMessageSend(room.DiscordChannel, "Failed to refresh credentials, unable to get XSTS token: "+err.Error())
			return
		}
		cache.Minecraft, err = GMMAuth.AuthMC(XSTSt)
		if err != nil {
			s.ChannelMessageSend(room.DiscordChannel, "Failed to refresh credentials, unable to get MC token: "+err.Error())
			return
		}
		profile, err := GMMAuth.GetMCprofile(cache.Minecraft.Token)
		if err != nil {
			s.ChannelMessageSend(room.DiscordChannel, "Unable to get MC profile: "+err.Error())
			return
		}
		cache.Username = profile.Name
		cache.UUID = profile.UUID
		err = writeCredentialsCache(room.AccountCredentialsName, cache)
		if err != nil {
			s.ChannelMessageSend(room.DiscordChannel, "Unable to write credentials cache: "+err.Error())
			return
		}
	}
	triggerChamber(s, room, cid, bot.Auth{Name: cache.Username, UUID: cache.UUID, AsTk: cache.Minecraft.Token})
}

func getPitchYaw(x0, y0, z0, x, y, z float64) (pitch, yaw float64) {
	dx := x - x0
	dy := y - y0
	dz := z - z0
	r := math.Sqrt(dx*dx + dy*dy + dz*dz)
	yaw = -math.Atan2(dx, dz) / math.Pi * 180
	if yaw < 0 {
		yaw = 360 + yaw
	}
	pitch = -math.Asin(dy/r) / math.Pi * 180
	return
}

func sendActivation(mcClient bot.Client, room PearlRoom, cid int) {
	blockCastPos := []float64{0.0, room.Chambers[cid].Pos[1] + 0.5, 0.0}
	if room.BotPos[0] < room.Chambers[cid].Pos[0] {
		blockCastPos[0] = room.Chambers[cid].Pos[0] - 0.5
	} else {
		blockCastPos[0] = room.Chambers[cid].Pos[0] + 0.5
	}
	if room.BotPos[2] < room.Chambers[cid].Pos[2] {
		blockCastPos[2] = room.Chambers[cid].Pos[2] + 0.5
	} else {
		blockCastPos[2] = room.Chambers[cid].Pos[2] - 0.5
	}
	_, yaw := getPitchYaw(room.BotPos[0], room.BotPos[1], room.BotPos[2],
		blockCastPos[0], blockCastPos[1], blockCastPos[2])
	mcClient.Conn.WritePacket(pk.Marshal(
		packetid.ServerboundMovePlayerRot,
		pk.Float(yaw),
		pk.Float(19.2),
		pk.Boolean(true),
	))
	cursorX := 0.0
	cursorZ := 0.0
	blockFace := 0
	if yaw > 315 || yaw < 45 {
		// facing south
		cursorX = 0.5
		cursorZ = 0.875
		blockFace = 2
	}
	if yaw > 45 && yaw < 135 {
		// facing west
		cursorX = 0.125
		cursorZ = 0.5
		blockFace = 5
	}
	if yaw > 135 && yaw < 225 {
		// facing north
		cursorX = 0.5
		cursorZ = 0.125
		blockFace = 3
	}
	if yaw > 225 && yaw < 315 {
		// facing east
		cursorX = 0.875
		cursorZ = 0.5
		blockFace = 4
	}
	time.Sleep(100 * time.Millisecond)
	log.Printf("yaw %.0f cursor %.2f %.2f block %.1f %.1f %.1f", yaw, cursorX, cursorZ, blockCastPos[0], blockCastPos[1], blockCastPos[2])
	log.Print(pk.Position{X: int(room.Chambers[cid].Pos[0]), Y: int(room.Chambers[cid].Pos[1]), Z: int(room.Chambers[cid].Pos[2])})
	mcClient.Conn.WritePacket(pk.Marshal(
		packetid.ServerboundUseItemOn,
		pk.VarInt(0), //hand
		pk.Position(pk.Position{X: int(room.Chambers[cid].Pos[0]), Y: int(room.Chambers[cid].Pos[1]), Z: int(room.Chambers[cid].Pos[2])}), //((int64(x)&0x3FFFFFF)<<38)|((int64(z)&0x3FFFFFF)<<12)|(int64(y)&0xFFF)), //position
		pk.VarInt(blockFace), //direction
		pk.Float(cursorX),    //cursor x
		pk.Float(0.125),      //y
		pk.Float(cursorZ),    //z
		pk.Boolean(true),     //inside
	))
	mcClient.Conn.WritePacket(pk.Marshal(
		packetid.ServerboundSwing,
		pk.VarInt(0), //hand
	))
}

func triggerChamber(s *discordgo.Session, room PearlRoom, cid int, auth bot.Auth) {
	mcClient := bot.NewClient()
	mcClient.Auth = auth
	mcPlayer := basic.NewPlayer(mcClient, basic.Settings{Locale: "en_US"})
	activateRoutine := func() {
		s.ChannelMessageSend(room.DiscordChannel, fmt.Sprintf("Logged in (%d), activating...", mcPlayer.Gamemode))
		time.Sleep(500 * time.Millisecond)
		if cid == -1 {
			for c := range room.Chambers {
				sendActivation(*mcClient, room, c)
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			sendActivation(*mcClient, room, cid)
		}
		s.ChannelMessageSend(room.DiscordChannel, "Activated.")
		time.Sleep(400 * time.Millisecond)
		mcClient.Close()
	}
	basic.EventsListener{
		GameStart: func() error {
			if mcPlayer.WorldInfo.HashedSeed == -4189754411863869379 {
				return nil
			}
			if mcPlayer.Gamemode == 0 {
				go activateRoutine()
			}
			return nil
		},
		ChatMsg: nil,
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
	go mcClient.HandleGame()
	time.Sleep(10000 * time.Millisecond)
	mcClient.Close()

	// mcClient.Conn.WritePacket(pk.Marshal(
	// 	0x14,
	// 	pk.Float(-106.0), //cursor x
	// 	pk.Float(34.0),   //y
	// 	pk.Boolean(true), //on ground
	// ))

}
