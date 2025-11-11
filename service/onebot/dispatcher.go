package onebot

import (
	"context"

	"sealchat/model"
	"sealchat/protocol/onebotv11"
)

type Dispatcher interface {
	HandleAction(ctx context.Context, profile *model.BotProfileModel, frame *onebotv11.ActionFrame) (*onebotv11.ActionResponse, error)
	HandleEvent(ctx context.Context, profile *model.BotProfileModel, event *onebotv11.Event) error
}

type nopDispatcher struct{}

func (nopDispatcher) HandleAction(_ context.Context, _ *model.BotProfileModel, frame *onebotv11.ActionFrame) (*onebotv11.ActionResponse, error) {
	return onebotv11.NewErrorResponse(frame.Echo, 1400, "action not implemented"), nil
}

func (nopDispatcher) HandleEvent(context.Context, *model.BotProfileModel, *onebotv11.Event) error {
	return nil
}

var dispatcher Dispatcher = nopDispatcher{}

func SetDispatcher(d Dispatcher) {
	if d == nil {
		dispatcher = nopDispatcher{}
		return
	}
	dispatcher = d
}

func getDispatcher() Dispatcher {
	if dispatcher == nil {
		return nopDispatcher{}
	}
	return dispatcher
}
