package historyops

import (
	"context"
	"desktop-app/api"
)

type Service struct {
	client *api.Client
}

func New(client *api.Client) *Service {
	return &Service{client: client}
}

func (s *Service) Load(ctx context.Context) ([]api.HistoryItem, error) {
	return s.client.GetHistory(ctx)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.client.DeleteHistoryItem(ctx, id)
}
