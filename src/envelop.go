package main

type Envelop struct {
	Sender    string
	Receiver  string
	Intent    string
	Payload   map[string]interface{}
	TraceID   string
	MessageID string
	ChannelID string
	TTL       int
	Meta      map[string]interface{}
}

func (e *Envelop) Clone() *Envelop {
	payloadClone := make(map[string]interface{})
	for k, v := range e.Payload {
		payloadClone[k] = v
	}
	metaClone := make(map[string]interface{})
	for k, v := range e.Meta {
		metaClone[k] = v
	}
	return &Envelop{
		Sender:    e.Sender,
		Receiver:  e.Receiver,
		Intent:    e.Intent,
		Payload:   payloadClone,
		TraceID:   e.TraceID,
		MessageID: e.MessageID,
		ChannelID: e.ChannelID,
		TTL:       e.TTL,
		Meta:      metaClone,
	}
}

type Plugin interface {
	Execute(env *Envelop, agent interface{}) *Envelop
}
