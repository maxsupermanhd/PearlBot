package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/bwmarrin/discordgo"
)

func commandStatus(s *discordgo.Session, i *discordgo.InteractionCreate) {
	PRreq, err := http.NewRequest("GET", "https://xnotify.xboxlive.com/servicestatusv6/"+config.StatusQueryRegion1+"/"+config.StatusQueryRegion2, nil)
	if err != nil {
		iTextResponse(s, i, "Failed to create request for XBL service status: "+err.Error())
		return
	}
	PRreq.Header.Add("Accept", "application/json")
	client := &http.Client{
		Timeout: 1 * time.Second,
	}
	PRresp, err := client.Do(PRreq)
	if err != nil {
		iTextResponse(s, i, "XBL service status request failed: "+err.Error())
		return
	}
	var PRres map[string]interface{}
	err = json.NewDecoder(PRresp.Body).Decode(&PRres)
	if err != nil {
		iTextResponse(s, i, "XBL service status responded with malformed JSON: "+err.Error())
		return
	}
	// https://xnotify.xboxlive.com/servicestatusv6/CA/en-CA
	// https://api.mojang.com/users/profiles/minecraft/Jeb
}
