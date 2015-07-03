package main

import (
	"github.com/nlopes/slack"
	"log"
	"math/rand"
	"os"
	"time"
)

var Questions = []string{
	"What did you do yesterday?",
	"What are you planning to do today?",
	"Are you blocked by anything? If so, what?",
	"How are you feeling?",
}

const StandupTimeMinutes = 4

var StandupNagMinuteDelays = []int{1, 2, 3}

const UserStandupStartText = "*WOOF!* Stand-up for #%s starting.\nMessage me `skip` to duck out of this one."
const UserStandupEndText = "Thanks! All done."
const UserStandupTimeUpText = "Too slow! The stand-up's finished now. Catch up in the channel."
const UserStandupAlreadyFinishedText = "Your next standup would have been for #%s but it's already finished. Catch up in the channel."
const UserNextStandupText = "But wait, you have another stand-up to attend…"
const UserConfirmSkipText = "Okay!"

var UserNagMessages = []string{
	"_nuzzle_ Don't forget me!",
	"_offers paw_ Do you have anything to say today?",
	"_wide puppy eyes_ Why are you so silent?",
	"Nudge. Nudgenudge.",
	"_stands right beside you, wagging tail so it thwocks against your leg_",
	"_paces around you_",
	"_drops the stand-up talking stick at your feet_",
}

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

	log.SetFlags(log.LstdFlags | log.Lshortfile)

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

func RandomisedNags() (out []string) {
	out = make([]string, len(UserNagMessages))
	copy(out, UserNagMessages)
	for i := range out {
		j := rand.Intn(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return
}
