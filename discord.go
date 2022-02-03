package main

import "github.com/bwmarrin/discordgo"

func iTextResponse(s *discordgo.Session, i *discordgo.InteractionCreate, resp string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: resp,
		},
	})
}
