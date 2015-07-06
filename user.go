package main

import (
	"fmt"
	"github.com/abourget/slack"
	"strings"
	"time"
)

/* Note there's a bit of a race between standups signalling that they've
 * timed out, the user advancing to the next standup in its queue,
 * and the Finished flag on a standup. It's a little ugly, but basically works.
 * Patches welcome.
 */

const userSkipCommand = "skip"

type User struct {
	Info               slack.User
	client             *AuthedSlack
	imChannelId        string
	events             chan userEvent
	standupQueue       []*Standup
	currentStandup     *Standup
	currentQuestionIdx int
	standupsFinished   map[*Standup]bool
	nagMinuteDelays    []int
	nagMessages        []string
	nagMessageIdx      int
	nagTimers          map[*time.Timer]bool
}

type userEvent interface {
	isUserEvent()
}

type userMessage slack.MessageEvent
type userNag struct{}

// tried to alias to the pointer type instead of wrapping in a struct, but
// go kept moaning at me and I couldn't work out why.
type userStartStandup struct {
	standup *Standup
}

type userStandupTimeUp struct {
	standup *Standup
}

type userEndStandup struct {
	standup *Standup
}

func (um userMessage) isUserEvent() {
}

func (un userNag) isUserEvent() {
}

func (s userStartStandup) isUserEvent() {
}

func (s userEndStandup) isUserEvent() {
}

func (s userStandupTimeUp) isUserEvent() {
}

func normaliseCommand(cmd string) string {
	return strings.ToLower(strings.TrimSpace(cmd))
}

func NewUser(client *AuthedSlack, info slack.User, imChannelId string) (u *User) {
	u = &User{
		Info:             info,
		client:           client,
		imChannelId:      imChannelId,
		events:           make(chan userEvent),
		standupQueue:     make([]*Standup, 0, 5),
		standupsFinished: make(map[*Standup]bool),
		nagMessages:      RandomisedNags(),
		nagMinuteDelays:  StandupNagMinuteDelays,
	}
	u.resetNags()
	go u.start()
	return
}

func (self *User) start() {
	for _, m := range self.nagMinuteDelays {
		nag := time.AfterFunc(time.Duration(m)*time.Minute, self.nag)
		self.nagTimers[nag] = true
	}

	for ei := range self.events {
		switch e := ei.(type) {
		case userMessage:
			if self.currentStandup != nil {
				if self.handleStandupCommand(e.Text) {
					continue
				}
				DebugLog.Printf("reporting message id %s as answer from %s", e.Id, self.Info.Id)
				self.currentStandup.ReportUserAnswer(self, self.currentQuestionIdx, e.Text)
				self.advanceQuestion()
			}

		case userStartStandup:
			s := e.standup
			s.ReportUserAcknowledged(self)

			if self.currentStandup == nil {
				self.startStandup(s)
			} else {
				self.standupQueue = append(self.standupQueue, s)
			}

		case userEndStandup:
			s := e.standup

			self.standupsFinished[s] = true

			if s == self.currentStandup {
				self.currentStandup = nil
				self.resetNags()

				next := self.popQueuedStandup()
				if next != nil {
					if self.standupsFinished[next] {
						self.standupAlreadyFinished(next)
					} else {
						self.sendIM(UserNextStandupText)
						self.startStandup(next)
					}
				}
			}

		case userNag:
			self.sendIM(self.nagMessages[self.nagMessageIdx])
			self.nagMessageIdx = (self.nagMessageIdx + 1) % len(self.nagMessages)

		case userStandupTimeUp:
			s := e.standup
			if s == self.currentStandup {
				self.sendIM(UserStandupTimeUpText)
			}
			self.endStandup(s)
		}
	}
}

func (self *User) StartStandup(s *Standup) {
	self.events <- userStartStandup{standup: s}
}

func (self *User) ReceiveMessageReply(m slack.MessageEvent) {
	self.events <- userMessage(m)
}

func (self *User) StandupTimeUp(s *Standup) {
	self.events <- userStandupTimeUp{standup: s}
}

func (self *User) sendIM(text string) {
	_, _, err := self.client.PostMessage(self.imChannelId, text, DefaultMessageParameters)
	if err != nil {
		self.handleError()
	}
}

func (self *User) handleStandupCommand(cmd string) bool {
	cmd = normaliseCommand(cmd)
	switch cmd {
	case userSkipCommand:
		self.currentStandup.ReportUserSkip(self)
		self.sendIM(UserConfirmSkipText)
		self.endCurrentStandup()
		return true
	}
	return false
}

func (self *User) advanceQuestion() {
	if self.currentStandup.IsLastQuestion(self.currentQuestionIdx) {
		self.sendIM(UserStandupEndText)
		self.endCurrentStandup()
	} else {
		self.currentQuestionIdx++
		go self.askCurrentQuestion()
	}
}

func (self *User) startStandup(s *Standup) {
	if s.Finished {
		self.standupAlreadyFinished(s)
		return
	}

	self.currentStandup = s
	self.currentQuestionIdx = 0

	go func() {
		self.sendIM(fmt.Sprintf(UserStandupStartText,
			self.currentStandup.Channel.Name))
		self.askCurrentQuestion()
	}()
}

func (self *User) endStandup(s *Standup) {
	go func() {
		self.events <- userEndStandup{standup: s}
	}()
}

func (self *User) endCurrentStandup() {
	self.endStandup(self.currentStandup)
}

func (self *User) popQueuedStandup() (s *Standup) {
	q := self.standupQueue
	if len(q) == 0 {
		return nil
	}
	s, self.standupQueue = q[len(q)-1], q[:len(q)-1]
	return
}

func (self *User) standupAlreadyFinished(s *Standup) {
	self.sendIM(fmt.Sprintf(
		UserStandupAlreadyFinishedText, s.Channel.Name))
	if self.standupsFinished[s] {
		delete(self.standupsFinished, s)
	}
}

func (self *User) nag() {
	self.events <- userNag{}
}

func (self *User) resetNags() {
	if self.nagTimers != nil {
		for nag, _ := range self.nagTimers {
			nag.Stop()
		}
	}
	self.nagTimers = make(map[*time.Timer]bool)
}

func (self *User) askCurrentQuestion() {
	self.sendIM(self.currentStandup.Questions[self.currentQuestionIdx])
}

func (self *User) handleError() {
	if self.currentStandup != nil {
		self.currentStandup.ReportUserError(self)
		self.endCurrentStandup()
	}
}
