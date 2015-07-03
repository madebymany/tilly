package main

import (
	"github.com/nlopes/slack"
	"log"
	"os"
	"time"
)

type Question struct {
	Text string
}

var Questions = []Question{
	Question{Text: "What did you do yesterday?"},
	Question{Text: "What are you planning to do today?"},
	Question{Text: "Are you blocked by anything? If so, what?"},
	Question{Text: "How are you feeling?"},
}

const StandupTimeMinutes = 1

var StandupNagMinuteDelays = []int{15, 5}

const UserStandupStartText = "*WOOF!* Stand-up for #%s starting.\nMessage me `skip` to duck out of this one."
const UserStandupEndText = "Thanks! All done."
const UserStandupTimeUpText = "Too slow! The stand-up's finished now. Catch up in the channel."
const UserStandupAlreadyFinishedText = "Your next standup would have been for #%s but it's already finished. Catch up in the channel."
const UserNextStandupText = "But wait, you have another stand-up to attend…"
const UserConfirmSkipText = "Okay!"

var DefaultMessageParameters = slack.PostMessageParameters{
	AsUser:      true,
	Markdown:    true,
	Parse:       "full",
	EscapeText:  true,
	UnfurlLinks: true,
	UnfurlMedia: true,
	LinkNames:   1,
}

type AuthedSlack struct {
	*slack.Slack
	UserId string
}

func main() {
	var err error

	slackToken := os.Getenv("SLACK_TOKEN")
	if slackToken == "" {
		log.Fatalln("You must provide a SLACK_TOKEN environment variable")
	}

	client := slack.New(slackToken)

	auth, err := client.AuthTest()
	if err != nil {
		log.Fatalf("Couldn't log in: %s", err)
	}
	authClient := &AuthedSlack{Slack: client, UserId: auth.UserId}

	slackWS, err := authClient.StartRTM("", "https://madebymany.slack.com")
	if err != nil {
		log.Fatalf("Couldn't start RTM: %s", err)
	}

	userManager := NewUserManager(authClient)
	eventReceiver := NewEventReceiver(slackWS, userManager, auth.UserId)
	go eventReceiver.Start()

	chs, err := authClient.GetChannels(true)
	if err != nil {
		log.Fatalf("Couldn't get channels: %s", err)
	}

	for _, ch := range chs {
		if ch.IsGeneral || !ch.IsMember {
			continue
		}

		go NewStandup(authClient, ch, userManager).Run()
	}

	for {
		// wait
		time.Sleep(time.Minute)
	}
}
