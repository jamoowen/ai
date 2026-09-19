package model

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Content interface {
	isContent()
}

type TextContent struct {
	Text string
}

func (TextContent) isContent() {}

type Message struct {
	Role    Role
	Content []Content
}

func NewUserMessage(text string) Message {
	return newTextMessage(RoleUser, text)
}

func NewAssistantMessage(text string) Message {
	return newTextMessage(RoleAssistant, text)
}

func newTextMessage(role Role, text string) Message {
	return Message{
		Role:    role,
		Content: []Content{TextContent{Text: text}},
	}
}
