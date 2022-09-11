package alerter

import (
	"os/exec"
	"strconv"
	"strings"
)

const (
	UserClickedNotification = "@CONTENTCLICKED"
)

type Notification struct {
	Title         string
	Subtitle      string
	Message       string
	actions       []string
	DropDownLabel string
	CloseLabel    string
	Timeout       int
	Sound         string
	GroupID       string
	//RemoveID      string
	//ListID        string
	//SenderID      string
	AppIcon      string
	ContentImage string
	reply        bool
}

func (n Notification) ToArgs() []string {
	res := make([]string, 0)
	if n.reply {
		res = append(res, "-reply")
	}
	if n.Title != "" {
		res = append(res, "-title", n.Title)
	}
	if n.Subtitle != "" {
		res = append(res, "-subtitle", n.Subtitle)
	}
	if n.Message != "" {
		res = append(res, "-message", n.Message)
	}
	if n.DropDownLabel != "" {
		res = append(res, "-dropdownLabel", n.DropDownLabel)
	}
	if n.CloseLabel != "" {
		res = append(res, "-closeLabel", n.CloseLabel)
	}
	if n.Timeout > 0 {
		res = append(res, "-timeout", strconv.Itoa(n.Timeout))
	}
	if n.Sound != "" {
		res = append(res, "-sound", n.Sound)
	}
	if n.AppIcon != "" {
		res = append(res, "-appIcon", n.AppIcon)
	}
	if n.ContentImage != "" {
		res = append(res, "-contentImage", n.ContentImage)
	}
	if len(n.actions) > 0 {
		res = append(res, "-actions")
		res = append(res, n.actions...)
	}
	return res
}

func Confirm(n *Notification) (bool, error) {
	n.CloseLabel = "No"
	n.actions = []string{"Yes"}
	res, err := exec.Command("alerter", n.ToArgs()...).Output()
	if err != nil {
		return false, err
	}

	answer := string(res)
	return strings.ToLower(answer) == "yes" || answer == UserClickedNotification, nil
}

func Reply(n *Notification) (string, error) {
	n.reply = true
	res, err := exec.Command("alerter", n.ToArgs()...).Output()
	if err != nil {
		return "", err
	}
	return string(res), nil
}

func MultipleChoice(n *Notification, actions []string) (string, error) {
	n.actions = actions
	res, err := exec.Command("alerter", n.ToArgs()...).Output()
	if err != nil {
		return "", err
	}
	return string(res), nil
}
