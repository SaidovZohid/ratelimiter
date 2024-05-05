package ratelimiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type LeakyBucket struct {
	Method         string `yaml:"method"`
	Path           string `yaml:"path"`
	RequestLimit   int    `yaml:"limit"`
	Interval       string `yaml:"interval"`
	Type           string `yaml:"type"`
	JWTKey         string `yaml:"jwt_key"`
	AllowOnFailure bool   `yaml:"allow_on_failure"`
}

type LeakyBucketI interface {
	AllowRequest(ctx context.Context, key string) bool
	GetJwtKey() string
	GetType() string
	GetAllowOnFailure() bool
}

type leakyBucketService struct {
	Method         string
	Path           string
	RequestLimit   int
	Interval       string
	Type           string
	JWTKey         string
	AllowOnFailure bool
	Id             int
	RedisClient    *redis.Client
}

func NewLeakyBucket(bucket *LeakyBucket, id int, redisClient *redis.Client) (LeakyBucketI, error) {
	message, ok := bucket.Validate()
	if !ok {
		return nil, fmt.Errorf("invalid LeakyBucket: %s", message)
	}

	return &leakyBucketService{
		Method:         bucket.Method,
		Path:           bucket.Path,
		RequestLimit:   bucket.RequestLimit,
		Interval:       bucket.Interval,
		JWTKey:         bucket.JWTKey,
		AllowOnFailure: bucket.AllowOnFailure,
		RedisClient:    redisClient,
		Type:           bucket.Type,
		Id:             id,
	}, nil
}

func (l *leakyBucketService) GetType() string {
	return l.Type
}

func (l *leakyBucketService) GetJwtKey() string {
	return l.JWTKey
}

func (l *leakyBucketService) GetAllowOnFailure() bool {
	return l.AllowOnFailure
}

func (l *leakyBucketService) AllowRequest(ctx context.Context, key string) bool {
	var (
		timeKey      = ""
		timeDuration = time.Second
	)

	switch l.Interval {
	case "second":
		timeKey = time.Now().Format("2006-01-02 15:04:05")
	case "minute":
		timeKey = time.Now().Format("2006-01-02 15:04")
		timeDuration = time.Minute
	case "hour":
		timeKey = time.Now().Format("2006-01-02 15")
		timeDuration = time.Hour
	}

	key = fmt.Sprintf("%s%d%s", timeKey, l.Id, key)

	result, err := l.RedisClient.Get(ctx, key).Int()
	if err == redis.Nil {
		l.RedisClient.Set(ctx, key, l.RequestLimit-1, timeDuration)
		return true
	} else if err != nil {
		return l.AllowOnFailure
	}

	if result <= 0 {
		return false
	}

	err = l.RedisClient.DecrBy(ctx, key, 1).Err()
	if err != nil {
		return l.AllowOnFailure
	}

	return true
}

func (l *LeakyBucket) Validate() (string, bool) {
	switch l.Method {
	case "GET", "POST", "PUT", "DELETE":
	default:
		return "Method must be one of GET, POST, PUT, DELETE", false
	}

	switch {
	case l.RequestLimit < 1:
		return "RequestLimit must be greater than 0", false
	case l.Interval != "second" && l.Interval != "minute" && l.Interval != "hour":
		return "DurationType must be one of second, minute, hour", false
	case l.Type != "ip" && l.Type != "jwt" && l.Type != "header" && l.Type != "query":
		return "Type must be one of ip, jwt, header, query", false
	}

	return "", true
}
