package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
	GMMAuth "github.com/maxsupermanhd/go-mc-ms-auth"
)

type AuthCache struct {
	Microsoft GMMAuth.MSauth `json:"microsoft"`
	Minecraft GMMAuth.MCauth `json:"minecraft"`
	Username  string         `json:"username"`
	UUID      string         `json:"uuid"`
}

type ErrorMicrosoftCacheExpired struct {
	Since time.Time
}

func (e *ErrorMicrosoftCacheExpired) Error() string {
	return fmt.Sprintf("Microsoft token expired %s ago", time.Since(e.Since).String())
}

type ErrorMinecraftCacheExpired struct {
	Since time.Time
}

func (e *ErrorMinecraftCacheExpired) Error() string {
	return fmt.Sprintf("Minecraft token expired %s ago", time.Since(e.Since).String())
}

func isDateExpired(d int64) bool {
	return d+2 <= time.Now().Unix()
}

func getCredentialsCache(path string) (out AuthCache, err error) {
	path = config.AccountsCredentialCachePath + path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return out, err
	} else {
		cachefile, err := os.Open(path)
		if err != nil {
			return out, err
		}
		defer cachefile.Close()
		cachecontent, err := ioutil.ReadAll(cachefile)
		if err != nil {
			return out, err
		}
		err = json.Unmarshal(cachecontent, &out)
		if err != nil {
			return out, err
		}
		return out, err
	}
}

func writeCredentialsCache(path string, cache AuthCache) error {
	path = config.AccountsCredentialCachePath + path
	cacheb, err := json.MarshalIndent(cache, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, cacheb, 0600)
}

func checkCredentialsValid(path string) (string, error) {
	path = config.AccountsCredentialCachePath + path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", err
	} else {
		cachefile, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer cachefile.Close()
		cachecontent, err := ioutil.ReadAll(cachefile)
		if err != nil {
			return "", err
		}
		var cache AuthCache
		err = json.Unmarshal(cachecontent, &cache)
		if err != nil {
			return "", err
		}
		if isDateExpired(cache.Microsoft.ExpiresAfter) {
			return cache.Username, &ErrorMicrosoftCacheExpired{time.Unix(cache.Microsoft.ExpiresAfter, 0)}
		}
		if isDateExpired(cache.Minecraft.ExpiresAfter) {
			return cache.Username, &ErrorMinecraftCacheExpired{time.Unix(cache.Minecraft.ExpiresAfter, 0)}
		}
		return cache.Username, nil
	}
}

func commandAuthCheck(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rooms := findRoomsByChannelID(i.ChannelID)
	if len(rooms) <= 0 {
		iTextResponse(s, i, "No room is registered in this channel")
		return
	}
	if len(rooms) == 1 {
		username, err := checkCredentialsValid(rooms[0].AccountCredentialsName)
		username = usernameBeautify(username)
		if err == nil {
			iTextResponse(s, i, "Credentials for room `"+rooms[0].RoomName+"` (account "+username+") active and cached")
		} else {
			iTextResponse(s, i, "Credentials for room `"+rooms[0].RoomName+"` (account "+username+"): "+err.Error())
		}
		return
	}
	resp := fmt.Sprintf("Registered rooms in this channel: %d\n", len(rooms))
	for i, r := range rooms {
		username, err := checkCredentialsValid(r.AccountCredentialsName)
		username = usernameBeautify(username)
		if err == nil {
			resp += fmt.Sprintf("[%d] `%s` - account %s's credentials are active and cached\n", i, r.RoomName, username)
		} else {
			resp += fmt.Sprintf("[%d] `%s` - account %s: %s\n", i, r.RoomName, username, err.Error())
		}
	}
	iTextResponse(s, i, resp)
}

func sliceConcat(in []string) (out string) {
	for _, i := range in {
		out += "\n" + i
	}
	return out
}

func tokenValidForString(d int64) string {
	return (time.Duration(d-time.Now().Unix()) * time.Second).String()
}

func commandAuthRefresh(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rooms := findRoomsByChannelID(i.ChannelID)
	var room PearlRoom
	if len(rooms) <= 0 {
		iTextResponse(s, i, "Channel does not have any rooms attached")
		return
	}
	if len(rooms) == 1 {
		room = rooms[0]
	} else {
		if len(i.ApplicationCommandData().Options[0].Options) < 1 {
			iTextResponse(s, i, "Multiple rooms registered, specify room name")
			return
		}
		roomname := i.ApplicationCommandData().Options[0].Options[0].StringValue()
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
	cache, err := getCredentialsCache(room.AccountCredentialsName)
	if err != nil {
		iTextResponse(s, i, "Failed to load credential cache: "+err.Error())
		return
	}
	resp := []string{"Account " + usernameBeautify(cache.Username), "", ""}
	responseSent := false
	if isDateExpired(cache.Microsoft.ExpiresAfter) {
		resp[1] = "`Microsoft` :arrows_counterclockwise: Refreshing..."
		iTextResponse(s, i, sliceConcat(resp))
		responseSent = true
		err := GMMAuth.CheckRefreshMS(&cache.Microsoft, config.MicrosoftCID)
		if err != nil {
			resp[1] = "`Microsoft` :interrobang: Refresh failed: " + err.Error()
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
			return
		}
		err = writeCredentialsCache(room.AccountCredentialsName, cache)
		if err != nil {
			resp[1] = "`Microsoft` :interrobang: Failed to save refreshed token: " + err.Error()
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
			return
		}
		resp[1] = "`Microsoft` :white_check_mark: Refreshed, will be active for " + tokenValidForString(cache.Microsoft.ExpiresAfter)
	} else {
		resp[1] = "`Microsoft` :white_check_mark: will be active for " + tokenValidForString(cache.Microsoft.ExpiresAfter)
	}
	if isDateExpired(cache.Minecraft.ExpiresAfter) {
		resp[2] = "`Minecraft` :arrows_counterclockwise: Refreshing..."
		if responseSent {
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
		} else {
			iTextResponse(s, i, sliceConcat(resp))
		}
		XBLa, err := GMMAuth.AuthXBL(cache.Microsoft.AccessToken)
		if err != nil {
			resp[2] = "`Minecraft` :interrobang: Refresh failed (XBL): " + err.Error()
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
			return
		}
		XSTSa, err := GMMAuth.AuthXSTS(XBLa)
		if err != nil {
			resp[2] = "`Minecraft` :interrobang: Refresh failed (XSTS): " + err.Error()
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
			return
		}
		MCa, err := GMMAuth.AuthMC(XSTSa)
		if err != nil {
			resp[2] = "`Minecraft` :interrobang: Refresh failed (MC): " + err.Error()
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
			return
		}
		resauth, err := GMMAuth.GetMCprofile(MCa.Token)
		if err != nil {
			resp[2] = "`Minecraft` :interrobang: Refresh failed (Profile): " + err.Error()
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
			return
		}
		cache.Minecraft = MCa
		cache.UUID = resauth.UUID
		cache.Username = resauth.Name
		err = writeCredentialsCache(room.AccountCredentialsName, cache)
		if err != nil {
			resp[2] = "`Minecraft` :interrobang: Refreshed, failed to write to cache: " + err.Error()
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
			return
		}
		resp[2] = "`Minecraft` :white_check_mark: Refreshed, will be active for " + tokenValidForString(cache.Minecraft.ExpiresAfter)
		s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
	} else {
		resp[2] = "`Minecraft` :white_check_mark: will be active for " + tokenValidForString(cache.Minecraft.ExpiresAfter)
		if responseSent {
			s.InteractionResponseEdit(s.State.User.ID, i.Interaction, &discordgo.WebhookEdit{Content: sliceConcat(resp)})
		} else {
			iTextResponse(s, i, sliceConcat(resp))
		}
	}
}

func commandAuthNew(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rooms := findRoomsByChannelID(i.ChannelID)
	var room PearlRoom
	if len(rooms) <= 0 {
		iTextResponse(s, i, "Channel does not have any rooms attached")
		return
	}
	if len(rooms) == 1 {
		room = rooms[0]
	} else {
		if len(i.ApplicationCommandData().Options[0].Options) < 1 {
			iTextResponse(s, i, "Multiple rooms registered, specify room name")
			return
		}
		roomname := i.ApplicationCommandData().Options[0].Options[0].StringValue()
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
	var auth GMMAuth.MSauth
	DeviceResp, err := http.PostForm("https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode", url.Values{
		"client_id": {config.MicrosoftCID},
		"scope":     {`XboxLive.signin offline_access`},
	})
	if err != nil {
		iTextResponse(s, i, "Error getting device code: "+err.Error())
		return
	}
	var DeviceRes map[string]interface{}
	err = json.NewDecoder(DeviceResp.Body).Decode(&DeviceRes)
	if err != nil {
		iTextResponse(s, i, "Error getting device code: "+err.Error())
		return
	}
	DeviceResp.Body.Close()
	if DeviceResp.StatusCode != 200 {
		iTextResponse(s, i, fmt.Sprintf("MS device request answered not HTTP200! Instead got %s and following json: %#v", DeviceResp.Status, DeviceRes))
		return
	}
	DeviceCode, ok := DeviceRes["device_code"].(string)
	if !ok {
		iTextResponse(s, i, "Device code not found in response")
		return
	}
	UserCode, ok := DeviceRes["user_code"].(string)
	if !ok {
		iTextResponse(s, i, "User code not found in response")
		return
	}
	VerificationURI, ok := DeviceRes["verification_uri"].(string)
	if !ok {
		iTextResponse(s, i, "Verification URI not found in response")
		return
	}
	ExpiresIn, ok := DeviceRes["expires_in"].(float64)
	if !ok {
		iTextResponse(s, i, "Expires In not found in response")
		return
	}
	PoolInterval, ok := DeviceRes["interval"].(float64)
	if !ok {
		iTextResponse(s, i, "Pooling interval not found in response")
		return
	}
	iTextResponse(s, i, "Authentication requested...")
	channel, err := s.UserChannelCreate(i.Member.User.ID)
	if err != nil {
		log.Println("error creating channel:", err)
	}
	s.ChannelMessageSend(channel.ID, "You are attempting authentication of a bot.\n"+
		"Your code is `"+UserCode+"`\nIt will expire in "+fmt.Sprintf("%f", ExpiresIn)+" seconds\n"+
		"Head over to Microsoft and authenticate: "+VerificationURI+"\nI will respond in channel if you finish authentication or error occurs")
	time.Sleep(4 * time.Second)

	for {
		time.Sleep(time.Duration(int(PoolInterval)+1) * time.Second)
		CodeResp, err := http.PostForm("https://login.microsoftonline.com/consumers/oauth2/v2.0/token", url.Values{
			"client_id":   {config.MicrosoftCID},
			"scope":       {"XboxLive.signin offline_access"},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {DeviceCode},
		})
		if err != nil {
			iTextResponse(s, i, "Error pooling auth: "+err.Error())
			return
		}
		var CodeRes map[string]interface{}
		err = json.NewDecoder(CodeResp.Body).Decode(&CodeRes)
		if err != nil {
			iTextResponse(s, i, "Error pooling auth: "+err.Error())
			return
		}
		CodeResp.Body.Close()
		if CodeResp.StatusCode == 400 {
			PoolError, ok := CodeRes["error"].(string)
			if !ok {
				iTextResponse(s, i, fmt.Sprintf("While pooling token got this unknown json: %#v", CodeRes))
				return
			}
			if PoolError == "authorization_pending" {
				continue
			}
			if PoolError == "authorization_declined" {
				iTextResponse(s, i, "Authentication was declined")
				return
			}
			if PoolError == "expired_token" {
				iTextResponse(s, i, "Authentication timed out")
				return
			}
			if PoolError == "invalid_grant" {
				iTextResponse(s, i, fmt.Sprintf("While pooling token got invalid_grant error: "+CodeRes["error_description"].(string)))
				return
			}
		} else if CodeResp.StatusCode == 200 {
			MSaccessToken, ok := CodeRes["access_token"].(string)
			if !ok {
				iTextResponse(s, i, "Access token not found in response")
				return
			}
			auth.AccessToken = MSaccessToken
			MSrefreshToken, ok := CodeRes["refresh_token"].(string)
			if !ok {
				iTextResponse(s, i, "Refresh token not found in response")
				return
			}
			auth.RefreshToken = MSrefreshToken
			MSexpireSeconds, ok := CodeRes["expires_in"].(float64)
			if !ok {
				iTextResponse(s, i, "Expires in not found in response")
				return
			}
			auth.ExpiresAfter = time.Now().Unix() + int64(MSexpireSeconds)
			break
		} else {
			iTextResponse(s, i, fmt.Sprintf("MS answered not HTTP200! Instead got %s and following json: %#v", CodeResp.Status, CodeRes))
			return
		}
	}
	if auth.AccessToken == "" {
		iTextResponse(s, i, "bug in msa loop")
		return
	}
	iTextResponse(s, i, "Microsoft authentication completed, getting Minecraft credentials...")
	XBLa, err := GMMAuth.AuthXBL(auth.AccessToken)
	if err != nil {
		iTextResponse(s, i, "Failed to get XBL token: "+err.Error())
		return
	}
	XSTSa, err := GMMAuth.AuthXSTS(XBLa)
	if err != nil {
		iTextResponse(s, i, "Failed to get XSTS token: "+err.Error())
		return
	}
	MCa, err := GMMAuth.AuthMC(XSTSa)
	if err != nil {
		iTextResponse(s, i, "Failed to get Minecraft token: "+err.Error())
		return
	}
	MCs, err := GMMAuth.GetMCprofile(MCa.Token)
	if err != nil {
		iTextResponse(s, i, "Failed to get Minecraft profile: "+err.Error())
		return
	}
	cache := AuthCache{
		Microsoft: auth,
		Minecraft: MCa,
		Username:  MCs.Name,
		UUID:      MCs.UUID,
	}
	iTextResponse(s, i, "Successfully authenticated with account `"+cache.Username+"` (UUID `"+cache.UUID+"`)")
	err = writeCredentialsCache(room.AccountCredentialsName, cache)
	if err != nil {
		iTextResponse(s, i, "Failed to store authentication! "+err.Error())
	}
}

func commandAuth(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// spew.Dump(i.ApplicationCommandData().Options)
	cmd := i.ApplicationCommandData().Options[0].Name
	if cmd == "check" {
		commandAuthCheck(s, i)
	}
	if cmd == "refresh" {
		commandAuthRefresh(s, i)
	}
	if cmd == "new" {
		commandAuthNew(s, i)
	}
	iTextResponse(s, i, "Allowed subcommands: check, refresh, new")
}
