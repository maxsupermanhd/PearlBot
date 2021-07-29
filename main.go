package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/webview/webview"
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

// AzureClientID Azure credentials
var AzureClientID string

// RCheckWindowURL regexp that get's first step code
var RCheckWindowURL = regexp.MustCompile(`^https://login\.microsoftonline\.com/common/oauth2/nativeclient\?code=(?P<code>\S+)`)

type MSauth struct {
	AccessToken  string
	ExpiresAfter int64
	RefreshToken string
}

func CheckRefreshMS(auth *MSauth) error {
	if auth.ExpiresAfter <= time.Now().Unix() {
		MSdata := url.Values{
			"client_id": {os.Getenv("AzureClientID")},
			// "client_secret": {os.Getenv("AzureSecret")},
			"refresh_token": {auth.RefreshToken},
			"grant_type":    {"refresh_token"},
			"redirect_uri":  {"https://login.microsoftonline.com/common/oauth2/nativeclient"},
		}
		MSresp, err := http.PostForm("https://login.live.com/oauth20_token.srf", MSdata)
		if err != nil {
			return err
		}
		var MSres map[string]interface{}
		json.NewDecoder(MSresp.Body).Decode(&MSres)
		MSresp.Body.Close()
		if MSresp.StatusCode != 200 {
			return fmt.Errorf("MS answered not HTTP200! Instead got %s and following json: %#v", MSresp.Status, MSres)
		}
		MSaccessToken, ok := MSres["access_token"].(string)
		if !ok {
			return errors.New("Access_token not found in response")
		}
		auth.AccessToken = MSaccessToken
		MSrefreshToken, ok := MSres["refresh_token"].(string)
		if !ok {
			return errors.New("Refresh_token not found in response")
		}
		auth.RefreshToken = MSrefreshToken
		MSexpireSeconds, ok := MSres["expires_in"].(float64)
		if !ok {
			return errors.New("Expires_in not found in response")
		}
		auth.ExpiresAfter = time.Now().Unix() + int64(MSexpireSeconds)
	}
	return nil
}

func AuthMSdevice() (MSauth, error) {
	var auth MSauth
	DeviceResp, err := http.PostForm("https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode", url.Values{
		"client_id": {os.Getenv("AzureClientID")},
		"scope":     {`XboxLive.signin offline_access`},
	})
	if err != nil {
		return auth, err
	}
	var DeviceRes map[string]interface{}
	json.NewDecoder(DeviceResp.Body).Decode(&DeviceRes)
	DeviceResp.Body.Close()
	if DeviceResp.StatusCode != 200 {
		return auth, fmt.Errorf("MS answered not HTTP200! Instead got %s and following json: %#v", DeviceResp.Status, DeviceRes)
	}
	DeviceCode, ok := DeviceRes["device_code"].(string)
	if !ok {
		return auth, errors.New("Device code not found in response")
	}
	UserCode, ok := DeviceRes["user_code"].(string)
	if !ok {
		return auth, errors.New("User code not found in response")
	}
	log.Print("User code: ", UserCode)
	VerificationURI, ok := DeviceRes["verification_uri"].(string)
	if !ok {
		return auth, errors.New("Verification URI not found in response")
	}
	log.Print("Verification URI: ", VerificationURI)
	ExpiresIn, ok := DeviceRes["expires_in"].(float64)
	if !ok {
		return auth, errors.New("Expires In not found in response")
	}
	log.Print("Expires in: ", ExpiresIn, " seconds")
	PoolInterval, ok := DeviceRes["interval"].(float64)
	if !ok {
		return auth, errors.New("Pooling interval not found in response")
	}
	UserMessage, ok := DeviceRes["message"].(string)
	if !ok {
		return auth, errors.New("Pooling interval not found in response")
	}
	log.Println(UserMessage)
	time.Sleep(4 * time.Second)

	for {
		time.Sleep(time.Duration(int(PoolInterval)+1) * time.Second)
		CodeResp, err := http.PostForm("https://login.microsoftonline.com/consumers/oauth2/v2.0/token", url.Values{
			"client_id":   {os.Getenv("AzureClientID")},
			"scope":       {"XboxLive.signin offline_access"},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {DeviceCode},
		})
		if err != nil {
			return auth, err
		}
		var CodeRes map[string]interface{}
		json.NewDecoder(CodeResp.Body).Decode(&CodeRes)
		CodeResp.Body.Close()
		if CodeResp.StatusCode == 400 {
			PoolError, ok := CodeRes["error"].(string)
			if !ok {
				return auth, fmt.Errorf("While pooling token got this unknown json: %#v", CodeRes)
			}
			if PoolError == "authorization_pending" {
				continue
			}
			if PoolError == "authorization_declined" {
				return auth, errors.New("User declined authorization")
			}
			if PoolError == "expired_token" {
				return auth, errors.New("Turns out " + strconv.Itoa(int(PoolInterval)) + " seconds is not enough to authorize user, go faster ma monkey")
			}
			if PoolError == "invalid_grant" {
				return auth, errors.New("While pooling token got invalid_grant error: " + CodeRes["error_description"].(string))
			}
		} else if CodeResp.StatusCode == 200 {
			MSaccessToken, ok := CodeRes["access_token"].(string)
			if !ok {
				return auth, errors.New("Access token not found in response")
			}
			auth.AccessToken = MSaccessToken
			MSrefreshToken, ok := CodeRes["refresh_token"].(string)
			if !ok {
				return auth, errors.New("Refresh token not found in response")
			}
			auth.RefreshToken = MSrefreshToken
			MSexpireSeconds, ok := CodeRes["expires_in"].(float64)
			if !ok {
				return auth, errors.New("Expires in not found in response")
			}
			auth.ExpiresAfter = time.Now().Unix() + int64(MSexpireSeconds)
			return auth, nil
		} else {
			return auth, fmt.Errorf("MS answered not HTTP200! Instead got %s and following json: %#v", CodeResp.Status, CodeRes)
		}
	}

}

func AuthMS(code string) (MSauth, error) {
	var auth MSauth
	MSdata := url.Values{
		"client_id": {os.Getenv("AzureClientID")},
		// "client_secret": {os.Getenv("AzureSecret")},
		"code":         {code},
		"grant_type":   {"authorization_code"},
		"redirect_uri": {"https://login.microsoftonline.com/common/oauth2/nativeclient"},
	}
	MSresp, err := http.PostForm("https://login.live.com/oauth20_token.srf", MSdata)
	if err != nil {
		return auth, err
	}
	var MSres map[string]interface{}
	json.NewDecoder(MSresp.Body).Decode(&MSres)
	MSresp.Body.Close()
	if MSresp.StatusCode != 200 {
		return auth, fmt.Errorf("MS answered not HTTP200! Instead got %s and following json: %#v", MSresp.Status, MSres)
	}
	MSaccessToken, ok := MSres["access_token"].(string)
	if !ok {
		return auth, errors.New("Access_token not found in response")
	}
	auth.AccessToken = MSaccessToken
	MSrefreshToken, ok := MSres["refresh_token"].(string)
	if !ok {
		return auth, errors.New("Refresh_token not found in response")
	}
	auth.RefreshToken = MSrefreshToken
	MSexpireSeconds, ok := MSres["expires_in"].(float64)
	if !ok {
		return auth, errors.New("Expires_in not found in response")
	}
	auth.ExpiresAfter = time.Now().Unix() + int64(MSexpireSeconds)
	return auth, nil
}

func AuthXBL(MStoken string) (string, error) {
	XBLdataMap := map[string]interface{}{
		"Properties": map[string]interface{}{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + MStoken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}
	XBLdata, err := json.Marshal(XBLdataMap)
	if err != nil {
		return "", err
	}
	XBLreq, err := http.NewRequest("POST", "https://user.auth.xboxlive.com/user/authenticate", bytes.NewBuffer(XBLdata))
	if err != nil {
		return "", err
	}
	XBLreq.Header.Set("Content-Type", "application/json")
	XBLreq.Header.Set("Accept", "application/json")
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Renegotiation:      tls.RenegotiateOnceAsClient,
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
	XBLresp, err := client.Do(XBLreq)
	if err != nil {
		return "", err
	}
	var XBLres map[string]interface{}
	json.NewDecoder(XBLresp.Body).Decode(&XBLres)
	XBLresp.Body.Close()
	if XBLresp.StatusCode != 200 {
		return "", fmt.Errorf("XBL answered not HTTP200! Instead got %s and following json: %#v", XBLresp.Status, XBLres)
	}
	XBLtoken, ok := XBLres["Token"].(string)
	if !ok {
		return "", errors.New("Token not found in XBL response")
	}
	return XBLtoken, nil
}

type XSTSauth struct {
	Token string
	UHS   string
}

func AuthXSTS(XBLtoken string) (XSTSauth, error) {
	var auth XSTSauth
	XSTSdataMap := map[string]interface{}{
		"Properties": map[string]interface{}{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{XBLtoken},
		},
		"RelyingParty": "rp://api.minecraftservices.com/",
		"TokenType":    "JWT",
	}
	XSTSdata, err := json.Marshal(XSTSdataMap)
	if err != nil {
		return auth, err
	}
	XSTSreq, err := http.NewRequest("POST", "https://xsts.auth.xboxlive.com/xsts/authorize", bytes.NewBuffer(XSTSdata))
	if err != nil {
		return auth, err
	}
	XSTSreq.Header.Set("Content-Type", "application/json")
	XSTSreq.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	XSTSresp, err := client.Do(XSTSreq)
	if err != nil {
		return auth, err
	}
	var XSTSres map[string]interface{}
	json.NewDecoder(XSTSresp.Body).Decode(&XSTSres)
	XSTSresp.Body.Close()
	if XSTSresp.StatusCode != 200 {
		return auth, fmt.Errorf("XSTS answered not HTTP200! Instead got %s and following json: %#v", XSTSresp.Status, XSTSres)
	}
	XSTStoken, ok := XSTSres["Token"].(string)
	if !ok {
		return auth, errors.New("Could not find Token in XSTS response")
	}
	auth.Token = XSTStoken
	XSTSdc, ok := XSTSres["DisplayClaims"].(map[string]interface{})
	if !ok {
		return auth, errors.New("Could not find DisplayClaims object in XSTS response")
	}
	XSTSxui, ok := XSTSdc["xui"].([]interface{})
	if !ok {
		return auth, errors.New("Could not find xui array in DisplayClaims object")
	}
	if len(XSTSxui) < 1 {
		return auth, errors.New("xui array in DisplayClaims object does not have any elements")
	}
	XSTSuhsObject, ok := XSTSxui[0].(map[string]interface{})
	if !ok {
		return auth, errors.New("Could not get ush object in xui array")
	}
	XSTSuhs, ok := XSTSuhsObject["uhs"].(string)
	if !ok {
		return auth, errors.New("Could not get uhs string from ush object")
	}
	auth.UHS = XSTSuhs
	return auth, nil
}

type MCauth struct {
	Token        string
	ExpiresAfter int64
}

func AuthMC(token XSTSauth) (MCauth, error) {
	var auth MCauth
	MCdataMap := map[string]interface{}{
		"identityToken": "XBL3.0 x=" + token.UHS + ";" + token.Token,
	}
	MCdata, err := json.Marshal(MCdataMap)
	if err != nil {
		return auth, err
	}
	MCreq, err := http.NewRequest("POST", "https://api.minecraftservices.com/authentication/login_with_xbox", bytes.NewBuffer(MCdata))
	if err != nil {
		return auth, err
	}
	MCreq.Header.Set("Content-Type", "application/json")
	MCreq.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	MCresp, err := client.Do(MCreq)
	if err != nil {
		return auth, err
	}
	var MCres map[string]interface{}
	json.NewDecoder(MCresp.Body).Decode(&MCres)
	MCresp.Body.Close()
	if MCresp.StatusCode != 200 {
		return auth, fmt.Errorf("MC answered not HTTP200! Instead got %s and following json: %#v", MCresp.Status, MCres)
	}
	MCtoken, ok := MCres["access_token"].(string)
	if !ok {
		return auth, errors.New("Could not find access_token in MC response")
	}
	auth.Token = MCtoken
	MCexpire, ok := MCres["expires_in"].(float64)
	if !ok {
		return auth, errors.New("Could not find expires_in in MC response")
	}
	auth.ExpiresAfter = time.Now().Unix() + int64(MCexpire)
	return auth, nil
}

type MCprofile struct {
	UUID string
	Name string
}

func GetMCprofile(token string) (MCprofile, error) {
	var profile MCprofile
	PRreq, err := http.NewRequest("GET", "https://api.minecraftservices.com/minecraft/profile", nil)
	if err != nil {
		return profile, err
	}
	PRreq.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	PRresp, err := client.Do(PRreq)
	if err != nil {
		return profile, err
	}
	var PRres map[string]interface{}
	json.NewDecoder(PRresp.Body).Decode(&PRres)
	PRresp.Body.Close()
	if PRresp.StatusCode != 200 {
		return profile, fmt.Errorf("MC (profile) answered not HTTP200! Instead got %s and following json: %#v", PRresp.Status, PRres)
	}
	PRuuid, ok := PRres["id"].(string)
	if !ok {
		return profile, errors.New("Could not find uuid in profile response")
	}
	profile.UUID = PRuuid
	PRname, ok := PRres["name"].(string)
	if !ok {
		return profile, errors.New("Could not find username in profile response")
	}
	profile.Name = PRname
	return profile, nil
}

func WebViewShitAuth() string {
	loginprompturl := "https://login.live.com/oauth20_authorize.srf?client_id=" + os.Getenv("AzureClientID") + "&response_type=code&redirect_uri=https://login.microsoftonline.com/common/oauth2/nativeclient&scope=XboxLive.signin%20offline_access"
	debug := true
	w := webview.New(debug)
	var oauthCode string
	w.SetTitle("Yo, log in to this totally legit window")
	w.SetSize(800, 600, webview.HintNone)
	w.Init(`window.onload = function() {OauthStepComplete(document.URL)}`)
	w.Bind("OauthStepComplete", func(loc string) error {
		if RCheckWindowURL.MatchString(loc) {
			oauthCode = RCheckWindowURL.FindStringSubmatch(loc)[1]
			log.Println("Got an authorization code, trying to get authorization token from MS...")
			w.Terminate()
		}
		return nil
	})
	w.Navigate(loginprompturl)
	w.Run()
	w.Destroy()
	return oauthCode
}

const CacheFilename = "./auth.cache"

func GetMCcredentials() (bot.Auth, error) {
	var resauth bot.Auth
	var MSa MSauth
	if _, err := os.Stat(CacheFilename); os.IsNotExist(err) {
		MSa, err := AuthMSdevice()
		if err != nil {
			return resauth, err
		}
		tocache, err := json.Marshal(MSa)
		if err != nil {
			return resauth, err
		}
		err = ioutil.WriteFile(CacheFilename, tocache, 0600)
		if err != nil {
			return resauth, err
		}
		log.Println("Got an authorization token, trying to authenticate XBL...")
	} else {
		cachefile, err := os.Open(CacheFilename)
		if err != nil {
			return resauth, err
		}
		defer cachefile.Close()
		cachecontent, err := ioutil.ReadAll(cachefile)
		if err != nil {
			return resauth, err
		}
		err = json.Unmarshal(cachecontent, &MSa)
		if err != nil {
			return resauth, err
		}
		MSaOld := MSa
		err = CheckRefreshMS(&MSa)
		if err != nil {
			return resauth, err
		}
		if MSaOld.AccessToken != MSa.AccessToken {
			tocache, err := json.Marshal(MSa)
			if err != nil {
				return resauth, err
			}
			err = ioutil.WriteFile(CacheFilename, tocache, 0600)
			if err != nil {
				return resauth, err
			}
		}
		log.Println("Got cached authorization token, trying to authenticate XBL...")
	}

	XBLa, err := AuthXBL(MSa.AccessToken)
	if err != nil {
		return resauth, err
	}
	log.Println("Authorized on XBL, trying to get XSTS token...")

	XSTSa, err := AuthXSTS(XBLa)
	if err != nil {
		return resauth, err
	}
	log.Println("Got XSTS token, trying to get MC token...")

	MCa, err := AuthMC(XSTSa)
	if err != nil {
		return resauth, err
	}
	log.Println("Got MC token, NOT checking that you own the game because it is too complicated and going straight for MC profile...")

	MCp, err := GetMCprofile(MCa.Token)
	if err != nil {
		return resauth, err
	}
	log.Println("Got MC profile")
	log.Println("UUID: " + MCp.UUID)
	log.Println("Name: " + MCp.Name)
	resauth.UUID = MCp.UUID
	resauth.Name = MCp.Name
	resauth.AsTk = MCa.Token
	return resauth, nil
}

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
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	log.Println("Roger, stopping shit.")
	dg.Close()
}

var McClient *bot.Client

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
	if cmd == "leave" {
		McClient.Close()
		s.ChannelMessageSend(m.ChannelID, "Left")
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
		mauth, err := GetMCcredentials()
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error auth: "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, "Authenticated as `"+mauth.Name+"` (`"+mauth.UUID+"`)")
		McClient = bot.NewClient()
		McClient.Auth = mauth
		_ = basic.NewPlayer(McClient, basic.Settings{Locale: "en_US"})
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
		}.Attach(McClient)
		err = McClient.JoinServer("simplyvanilla.net")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error auth: "+err.Error())
			return
		}
		s.ChannelMessageSend(m.ChannelID, "Logged in")
		go McClient.HandleGame()

		time.Sleep(1 * time.Second)
		// McClient.Conn.WritePacket(pk.Marshal(
		// 	0x14,
		// 	pk.Float(-106.0), //cursor x
		// 	pk.Float(34.0),   //y
		// 	pk.Boolean(true), //on ground
		// ))
		McClient.Conn.WritePacket(pk.Marshal(
			packetid.BlockPlace,
			pk.VarInt(0), //hand
			pk.Position(pk.Position{X: x, Y: y, Z: z}), //((int64(x)&0x3FFFFFF)<<38)|((int64(z)&0x3FFFFFF)<<12)|(int64(y)&0xFFF)), //position
			pk.VarInt(4),      //direction
			pk.Float(0.836),   //cursor x
			pk.Float(0.187),   //y
			pk.Float(0.5),     //z
			pk.Boolean(false), //inside
		))
		McClient.Conn.WritePacket(pk.Marshal(
			packetid.ArmAnimation,
			pk.VarInt(0), //hand
		))
		s.ChannelMessageSend(m.ChannelID, "Activated.")
		time.Sleep(400 * time.Millisecond)
		McClient.Close()
		return
	} else if cmd == "show" {
		msg := fmt.Sprintf("Chamber %d registered at X%d Y%d Z%d", selected.Index, selected.X, selected.Y, selected.Z)
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}
}

func ActivateTrapdoor(pos pk.Position) {
	time.Sleep(1 * time.Second)
	// McClient.Conn.WritePacket(pk.Marshal(
	// 	0x14,
	// 	pk.Float(-106.0), //cursor x
	// 	pk.Float(34.0),   //y
	// 	pk.Boolean(true), //on ground
	// ))
	McClient.Conn.WritePacket(pk.Marshal(
		packetid.BlockPlace,
		pk.VarInt(0), //hand
		pk.Position(pk.Position{X: x, Y: y, Z: z}), //((int64(x)&0x3FFFFFF)<<38)|((int64(z)&0x3FFFFFF)<<12)|(int64(y)&0xFFF)), //position
		pk.VarInt(4),      //direction
		pk.Float(0.836),   //cursor x
		pk.Float(0.187),   //y
		pk.Float(0.5),     //z
		pk.Boolean(false), //inside
	))
	McClient.Conn.WritePacket(pk.Marshal(
		packetid.ArmAnimation,
		pk.VarInt(0), //hand
	))
	time.Sleep(400 * time.Millisecond)
	McClient.Close()
}

func onGameStart() error { // -106 34
	// McClient.Close()
	return nil
}

func onChatMsg(c chat.Message, pos byte, uuid uuid.UUID) error {
	log.Println("Chat:", c)
	return nil
}

func onDeath() error {
	log.Println("Died")
	// If we exclude Respawn(...) then the player won't press the "Respawn" button upon death
	return nil //p.Respawn()
}

func onDisconnect(c chat.Message) error {
	log.Println("Disconnect:", c)
	return nil
}
