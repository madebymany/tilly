package main

import (
	"bytes"
	"fmt"
	"github.com/abourget/slack"
	"sync"
	"time"
)

type Standup struct {
	Questions         []string
	Finished          bool
	Channel           slack.Channel
	Duration          time.Duration
	client            *AuthedSlack
	userIds           []string
	userManager       *UserManager
	userReplies       map[*User]userReply
	userRepliesMutex  sync.Mutex
	finishedChan      chan struct{}
	reportedWaitGroup *sync.WaitGroup
}

type userReply interface {
	isUserReply()
}

type userAbsentReply struct{}
type userAnswersReply []string
type userSkippedReply struct{}
type userErrorReply struct{}

func (r userAbsentReply) isUserReply() {
}

func (r userAnswersReply) isUserReply() {
}

func (r userAnswersReply) isCompleted() bool {
	for _, a := range r {
		if a == "" {
			return false
		}
	}
	return true
}

func (r userSkippedReply) isUserReply() {
}

func (r userErrorReply) isUserReply() {
}

func NewStandup(client *AuthedSlack, channel slack.Channel, userManager *UserManager, reportedWaitGroup *sync.WaitGroup) (s *Standup) {

	reportedWaitGroup.Add(1)

	s = &Standup{
		client:            client,
		Channel:           channel,
		userManager:       userManager,
		userReplies:       make(map[*User]userReply),
		Questions:         Questions,
		finishedChan:      make(chan struct{}, 1),
		Duration:          StandupTimeMinutes * time.Minute,
		reportedWaitGroup: reportedWaitGroup,
	}

	return s
}

func (self *Standup) Run() {
	self.userIds = make([]string, 0, len(self.Channel.Members))

	for _, userId := range self.Channel.Members {
		if userId != self.client.UserId && self.userManager.StartStandup(self, userId) {
			self.userIds = append(self.userIds, userId)
		}
	}

	go self.startTheClock()

	_ = <-self.finishedChan
	self.Finished = true
	DebugLog.Print("sending summary...")

	var msg bytes.Buffer

	msg.WriteString("<!everyone>: *BARKBARKBARK Stand-up done!*\nQuestions were:\n")
	for _, q := range self.Questions {
		msg.WriteString("• ")
		msg.WriteString(q)
		msg.WriteString("\n")
	}
	msg.WriteString("\n")

	for user, anyReply := range self.userReplies {
		userName := fmt.Sprintf("<@%s|%s>", user.Info.Id, user.Info.Name)
		switch reply := anyReply.(type) {
		case userAnswersReply:
			msg.WriteString(userName)
			msg.WriteString(" answered:\n")
			for _, a := range reply {
				if a == "" {
					msg.WriteString("but didn't respond to the rest.\n")
					break
				}
				msg.WriteString("• ")
				msg.WriteString(a)
				msg.WriteString("\n")
			}
		case userAbsentReply:
			msg.WriteString(userName)
			msg.WriteString(" never replied to me :disappointed:")
		case userSkippedReply:
			msg.WriteString(userName)
			msg.WriteString(" skipped this stand-up.")
		case userErrorReply:
			msg.WriteString("There was an error when trying to chat with ")
			msg.WriteString(userName)
		default:
			msg.WriteString("I don't know what ")
			msg.WriteString(userName)
			msg.WriteString(" did. It is a mystery to me. :no_mouth:")
		}
		msg.WriteString("\n")
	}

	var params slack.PostMessageParameters
	params = DefaultMessageParameters
	params.Parse = "none"
	params.LinkNames = 0
	params.EscapeText = false

	_, _, err := self.client.PostMessage(self.Channel.Id, msg.String(), params)
	if err == nil {
		DebugLog.Print("summary sent")
	} else {
		DebugLog.Printf("error posting summary: %s", err.Error())
	}

	self.reportedWaitGroup.Done()
}

func (self *Standup) ReportUserAcknowledged(u *User) {
	self.userRepliesMutex.Lock()
	defer self.userRepliesMutex.Unlock()

	self.userReplies[u] = userAbsentReply{}
	// don't check for completion, we're only just starting
}

func (self *Standup) ReportUserAnswer(u *User, qidx int, answer string) {
	self.userRepliesMutex.Lock()
	defer self.userRepliesMutex.Unlock()

	DebugLog.Printf("got answer from user %s: %s", u.Info.Name, answer)
	reply, replyExists := self.userReplies[u]
	if _, isAbsent := reply.(userAbsentReply); !replyExists || isAbsent {
		reply = make(userAnswersReply, len(self.Questions))
		self.userReplies[u] = reply
	}
	if answers, ok := reply.(userAnswersReply); ok {
		answers[qidx] = answer
	}

	self.checkFinished()
}

func (self *Standup) ReportUserError(u *User) {
	self.userRepliesMutex.Lock()
	defer self.userRepliesMutex.Unlock()

	self.userReplies[u] = userErrorReply{}
	self.checkFinished()
}

func (self *Standup) ReportUserSkip(u *User) {
	self.userRepliesMutex.Lock()
	defer self.userRepliesMutex.Unlock()

	self.userReplies[u] = userSkippedReply{}
	self.checkFinished()
}

func (self *Standup) IsLastQuestion(i int) bool {
	return i >= len(self.Questions)-1
}

func (self *Standup) startTheClock() {
	time.Sleep(self.Duration)

	self.userRepliesMutex.Lock()
	defer self.userRepliesMutex.Unlock()

	for user, _ := range self.userReplies {
		user.StandupTimeUp(self)
	}

	self.finish()
}

func (self *Standup) finish() {
	DebugLog.Print("finishing standup...")
	self.finishedChan <- struct{}{}
}

func (self *Standup) isFinished() bool {
	if len(self.userIds) != len(self.userReplies) {
		return false
	}
	for _, reply := range self.userReplies {
		switch r := reply.(type) {
		case userAnswersReply:
			if !r.isCompleted() {
				return false
			}
		case userAbsentReply:
			return false
		}
	}
	return true
}

func (self *Standup) checkFinished() {
	if self.isFinished() {
		self.finish()
	}
}
