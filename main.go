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

const UserStandupStartText = "WOOF! Standup for %s starting."
const UserStandupEndText = "Thanks! All done."

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

		s := NewStandup(authClient, ch, userManager)
		s.Run()
	}

	// hack for now
	time.Sleep(time.Second)
}
