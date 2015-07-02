package main

import (
	"fmt"
	"github.com/nlopes/slack"
)

const (
	UserStateReady int = iota
	UserStateWaiting
)

type User struct {
	Info               slack.User
	client             *AuthedSlack
	imChannelId        string
	messageReplies     chan slack.MessageEvent
	queueStandup       chan *Standup
	standupQueue       []*Standup
	currentStandup     *Standup
	currentQuestionIdx int
}

func NewUser(client *AuthedSlack, info slack.User, imChannelId string) (u *User) {
	u = &User{
		Info:           info,
		client:         client,
		imChannelId:    imChannelId,
		messageReplies: make(chan slack.MessageEvent),
		queueStandup:   make(chan *Standup),
		standupQueue:   make([]*Standup, 0, 5),
	}
	go u.start()
	return
}

func (self *User) start() {
	for {
		select {
		case m := <-self.messageReplies:
			if self.currentStandup != nil {
				self.currentStandup.ReportUserReply(self, self.currentQuestionIdx, m.Text)
				self.advanceQuestion()
			}
		case s := <-self.queueStandup:
			if self.currentStandup == nil {
				self.startStandup(s)
			} else {
				self.standupQueue = append(self.standupQueue, s)
			}
		}
	}
}

func (self *User) StartStandup(s *Standup) {
	self.queueStandup <- s
}

func (self *User) ReceiveMessageReply(m slack.MessageEvent) {
	self.messageReplies <- m
}

func (self *User) sendIM(text string) {
	_, _, err := self.client.PostMessage(self.imChannelId, text, DefaultMessageParameters)
	if err != nil {
		self.currentStandup.ReportUserError(self)
	}
}

func (self *User) advanceQuestion() {
	if self.currentStandup.IsLastQuestion(self.currentQuestionIdx) {
		self.endCurrentStandup()
	} else {
		self.currentQuestionIdx++
		self.askCurrentQuestion()
	}
}

func (self *User) startStandup(s *Standup) {
	self.currentStandup = s
	self.currentQuestionIdx = 0

	self.sendIM(fmt.Sprintf(UserStandupStartText,
		self.currentStandup.Channel.Name))
	self.askCurrentQuestion()
}

func (self *User) endCurrentStandup() {
	self.currentStandup = nil
	self.sendIM(UserStandupEndText)
	next := self.popQueuedStandup()
	if next != nil {
		self.startStandup(next)
	}
}

func (self *User) popQueuedStandup() (s *Standup) {
	q := self.standupQueue
	if len(q) == 0 {
		return nil
	}
	s, self.standupQueue = q[len(q)-1], q[:len(q)-1]
	return
}

func (self *User) askCurrentQuestion() {
	self.sendIM(self.currentStandup.Questions[self.currentQuestionIdx].Text)
}

func (self *User) handleError() {
	if self.currentStandup != nil {
		self.currentStandup.ReportUserError(self)
	}
}
