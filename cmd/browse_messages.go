package cmd

import (
	"github.com/syasika/miniaws/internal/s3ops"
	"github.com/syasika/miniaws/internal/sqsops"
)

type containerStatusMsg struct {
	status string
	name   string
}

type bucketsMsg struct {
	buckets []string
}

type bucketsErrMsg struct {
	err string
}

type objectsMsg struct {
	objects []s3ops.Object
}

type objectsErrMsg struct {
	err string
}

type resultMsg struct {
	desc    string
	refresh bool
}

type actionMsg struct {
	description string
}

type errMsg struct {
	err error
}

type paramsMsg struct {
	params       []ssmParam
	requestToken string
	nextToken    string
	label        string
}

type paramsErrMsg struct {
	err string
}

type paramValueMsg struct {
	desc string
}

type queuesMsg struct {
	queues []sqsops.Queue
}

type queuesErrMsg struct {
	err string
}

type messagesMsg struct {
	messages []sqsops.Message
}

type messagesErrMsg struct {
	err string
}
