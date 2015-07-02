package main

import (
	"bytes"
	"github.com/nlopes/slack"
	"sync"
)

type Standup struct {
	Questions        []Question
	client           *AuthedSlack
	Channel          slack.Channel
	userIds          []string
	userManager      *UserManager
	userReplies      map[*User][]string
	userRepliesMutex sync.Mutex
	finished         chan struct{}
}

func NewStandup(client *AuthedSlack, channel slack.Channel, userManager *UserManager) (s *Standup) {
	s = &Standup{
		client:      client,
		Channel:     channel,
		userManager: userManager,
		userReplies: make(map[*User][]string),
		Questions:   Questions,
		finished:    make(chan struct{}, 1),
	}

	s.userIds = make([]string, 0, len(s.Channel.Members))
	for _, userId := range s.Channel.Members {
		if userId != s.client.UserId {
			s.userIds = append(s.userIds, userId)
		}
	}

	return s
}

func (self *Standup) Run() {
	for _, userId := range self.userIds {
		self.userManager.StartStandup(self, userId)
	}
	_ = <-self.finished

	var msg bytes.Buffer

	msg.WriteString("@channel: *Standup done!*\nQuestions were:\n")
	for _, q := range self.Questions {
		msg.WriteString("• ")
		msg.WriteString(q.Text)
		msg.WriteString("\n")
	}
	msg.WriteString("\n")

	for user, answers := range self.userReplies {
		msg.WriteString(user.Info.RealName)
		msg.WriteString(" answered:\n")
		for _, a := range answers {
			msg.WriteString("• ")
			msg.WriteString(a)
			msg.WriteString("\n")
		}
	}

	self.client.PostMessage(self.Channel.Id, msg.String(), DefaultMessageParameters)
}

func (self *Standup) ReportUserReply(u *User, qidx int, answer string) {
	self.userRepliesMutex.Lock()
	defer self.userRepliesMutex.Unlock()

	replies, ok := self.userReplies[u]
	if !ok {
		replies = make([]string, len(self.Questions))
		self.userReplies[u] = replies
	}
	replies[qidx] = answer

	self.checkFinished()
}

func (self *Standup) ReportUserError(u *User) {

}

func (self *Standup) IsLastQuestion(i int) bool {
	return i >= len(self.Questions)-1
}

func (self *Standup) isFinished() bool {
	if len(self.userIds) != len(self.userReplies) {
		return false
	}
	for _, answers := range self.userReplies {
		for _, a := range answers {
			if a == "" {
				return false
			}
		}
	}
	return true
}

func (self *Standup) checkFinished() {
	if self.isFinished() {
		self.finished <- struct{}{}
	}
}
