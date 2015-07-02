package main

import (
	"github.com/nlopes/slack"
)

type EventReceiver struct {
	client      *slack.SlackWS
	userManager *UserManager
	events      chan slack.SlackEvent
	botUserId   string
}

func NewEventReceiver(client *slack.SlackWS, um *UserManager, botUserId string) (er *EventReceiver) {
	return &EventReceiver{
		client:      client,
		userManager: um,
		events:      make(chan slack.SlackEvent),
		botUserId:   botUserId,
	}
}

func (self *EventReceiver) Start() {
	go self.client.HandleIncomingEvents(self.events)
	for ev := range self.events {
		if m, ok := ev.Data.(*slack.MessageEvent); ok && m.UserId != self.botUserId {
			self.userManager.ReceiveMessageReply(*m)
		}
	}
}
