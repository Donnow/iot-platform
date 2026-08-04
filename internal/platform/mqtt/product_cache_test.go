package mqtt

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
)

func TestProductCacheHitAvoidsLoader(t *testing.T) {
	var loads int32
	cache := newProductCache(func(_ context.Context, key string) (domain.Product, error) {
		atomic.AddInt32(&loads, 1)
		return domain.Product{ProductKey: key, Name: "P"}, nil
	}, time.Minute)
	ctx := context.Background()
	product, err := cache.Get(ctx, "pk")
	if err != nil || product.ProductKey != "pk" {
		t.Fatalf("product=%#v err=%v", product, err)
	}
	product, err = cache.Get(ctx, "pk")
	if err != nil || product.ProductKey != "pk" {
		t.Fatalf("cached product=%#v err=%v", product, err)
	}
	if got := atomic.LoadInt32(&loads); got != 1 {
		t.Fatalf("expected 1 loader call after a cache hit, got %d", got)
	}
}

func TestProductCacheExpires(t *testing.T) {
	var loads int32
	cache := newProductCache(func(_ context.Context, key string) (domain.Product, error) {
		atomic.AddInt32(&loads, 1)
		return domain.Product{ProductKey: key, Name: "P"}, nil
	}, 10*time.Millisecond)
	ctx := context.Background()
	_, _ = cache.Get(ctx, "pk")
	time.Sleep(20 * time.Millisecond)
	_, _ = cache.Get(ctx, "pk")
	if got := atomic.LoadInt32(&loads); got != 2 {
		t.Fatalf("expected 2 loader calls after expiry, got %d", got)
	}
}

func TestProductCacheDoesNotCacheErrors(t *testing.T) {
	cache := newProductCache(func(_ context.Context, _ string) (domain.Product, error) {
		return domain.Product{}, errors.New("product not found")
	}, time.Minute)
	if _, err := cache.Get(context.Background(), "missing"); err == nil {
		t.Fatal("expected loader error to propagate")
	}
}
