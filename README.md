# PearlBot

Automatic stasis activation from Discord

## Features

- Authentication via discord message
- Stores and manages credentials
- Automatically refreshes tokens when needed
- Multiple accounts support
- Multiple "pearl rooms" support (even in same channel)
- Reliable activation
- Config hotsave/hotload

## Setup

Requires Go 1.18

```bash
git clone https://github.com/maxsupermanhd/PearlBot.git
cd PearlBot
go build
./PearlBot
```

Feel free to wrap it into service or run it in tmux/screen

## Commands

`/activate <chamber> [room]` - refreshes required tokens, logs in and activates stasis chamber\
`/auth check` - displays overview of all stored credentials/tokens\
`/auth new [room]` - initiates Microsoft device login flow, writes down credentials/tokens to a file\
`/auth refresh [room]` - initiates force token refresh\
`/config save/load` - loads or saves configuration to file\
`/help` - in case you have amnesia\
`/rooms` - displays registered rooms overview

## License

GNU Affero General Public License v3.0
