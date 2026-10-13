package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func postgresRepos(pool *pgxpool.Pool) repos {
	return repos{
		vouchers:      store.NewPostgresVoucherRepository(pool),
		reservations:  store.NewPostgresReservationRepository(pool),
		flashSales:    store.NewPostgresFlashSaleRepository(pool),
		subscriptions: store.NewPostgresSubscriptionRepository(pool),
		adCampaigns:   store.NewPostgresAdCampaignRepository(pool),
	}
}

func inMemoryRepos() repos {
	return repos{
		vouchers:      store.NewInMemoryVoucherRepository(),
		reservations:  store.NewInMemoryReservationRepository(),
		flashSales:    store.NewInMemoryFlashSaleRepository(),
		subscriptions: store.NewInMemorySubscriptionRepository(),
		adCampaigns:   store.NewInMemoryAdCampaignRepository(),
	}
}
