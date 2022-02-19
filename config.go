package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/bwmarrin/discordgo"
)

type Chamber struct {
	Index int       `json:"index"`
	Pos   []float64 `json:"pos"`
}
type PearlRoom struct {
	Chambers               []Chamber `json:"chambers"`
	AccountOwner           string    `json:"accountOwnerDiscordId"`
	AccountCredentialsName string    `json:"accountCredentialsName"`
	DiscordChannel         string    `json:"discordChannel"`
	RoomName               string    `json:"roomName"`
	ServerAdress           string    `json:"serverAdress"`
	BotPos                 []float64 `json:"botPos"`
}
type BotConfiguration struct {
	RemoveUnmetMessages         bool        `json:"removeUnmet"`
	RemoveUnmetIgnore           []string    `json:"removeIgnore"`
	PearlRooms                  []PearlRoom `json:"rooms"`
	DiscordToken                string      `json:"discordToken"`
	DiscordServiceChannel       string      `json:"discordServiceChannel"`
	AccountsCredentialCachePath string      `json:"accountsCredentialsCachePath"`
	MicrosoftCID                string      `json:"microsoftCID"`
	GuildID                     string      `json:"guildID"`
	ApplicationID               string      `json:"applicationID"`
	StatusQueryRegion1          string      `json:"statusRegion1"`
	StatusQueryRegion2          string      `json:"statusRegion2"`
	ReInitCommands              bool        `json:"reinitCommands"`
}

var (
	config *BotConfiguration
)

const (
	configPath string = "./config.json"
)

func loadConfig() error {
	configf, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer configf.Close()
	configb, err := ioutil.ReadAll(configf)
	if err != nil {
		return err
	}
	var conf BotConfiguration
	err = json.Unmarshal(configb, &conf)
	if err != nil {
		return err
	}
	config = &conf
	return nil
}

func saveConfig() error {
	conf, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile("./config.json", conf, 0664)
}

func verifyConfig() error {
	for i, c := range config.PearlRooms {
		sharedChannel := false
		for ii := i + 1; ii < len(config.PearlRooms); ii++ {
			cc := config.PearlRooms[ii]
			if c.AccountCredentialsName == cc.AccountCredentialsName {
				return fmt.Errorf("room %s and %s have same account credentials name",
					c.AccountCredentialsName, cc.AccountCredentialsName)
			}
			if c.DiscordChannel == cc.DiscordChannel {
				sharedChannel = true
			}
			if c.DiscordChannel == cc.DiscordChannel && c.RoomName == cc.RoomName {
				return fmt.Errorf("room %s have same name and discord channel, remove or rename", c.RoomName)
			}
		}
		if c.RoomName == "" && sharedChannel {
			return fmt.Errorf("empty pearl room name assigned to shared channel")
		}
		if len(c.BotPos) != 3 {
			return fmt.Errorf("bot position is not 3 floats")
		}
	}
	return nil
}

func commandConfig(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cmd := i.ApplicationCommandData().Options[0].Name
	if cmd == "load" {
		err := loadConfig()
		if err != nil {
			iTextResponse(s, i, "Error loading config: "+err.Error())
			return
		}
		err = verifyConfig()
		if err != nil {
			iTextResponse(s, i, "Error verifying config: "+err.Error())
			return
		}
		iTextResponse(s, i, "Config loaded.")
	}
	if cmd == "save" {
		err := saveConfig()
		if err != nil {
			iTextResponse(s, i, "Error saving config: "+err.Error())
			return
		}
		iTextResponse(s, i, "Config saved.")
	}
	iTextResponse(s, i, "Usage: `/config (load|save)`")
}
