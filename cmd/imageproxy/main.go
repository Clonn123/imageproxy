// Copyright 2013 The imageproxy authors.
// SPDX-License-Identifier: Apache-2.0

// imageproxy starts an HTTP server that proxies requests for remote images.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PaulARoy/azurestoragecache"
	"github.com/die-net/lrucache"
	"github.com/die-net/lrucache/twotier"
	"github.com/gomodule/redigo/redis"
	"github.com/gregjones/httpcache/diskcache"
	rediscache "github.com/gregjones/httpcache/redis"
	"github.com/peterbourgon/diskv"
	"willnorris.com/go/imageproxy"
	"willnorris.com/go/imageproxy/internal/gcscache"
	"willnorris.com/go/imageproxy/internal/s3cache"
	"willnorris.com/go/imageproxy/third_party/envy"
)

const defaultMemorySize = 100

var addr = flag.String("addr", "localhost:8080", "address to listen on, either a TCP address or a Unix domain socket path prefixed with unix:")
var allowHosts = flag.String("allowHosts", "", "comma separated list of allowed remote hosts")
var denyHosts = flag.String("denyHosts", "", "comma separated list of denied remote hosts")
var referrers = flag.String("referrers", "", "comma separated list of allowed referring hosts")
var includeReferer = flag.Bool("includeReferer", false, "include referer header in remote requests")
var followRedirects = flag.Bool("followRedirects", true, "follow redirects")
var baseURL = flag.String("baseURL", "", "default base URL for relative remote URLs")
var passRequestHeaders = flag.String("passRequestHeaders", "", "comma separatetd list of request headers to pass to remote server")
var passResponseHeaders = flag.String("passResponseHeaders", "Cache-Control,Last-Modified,Expires,Etag,Link", "comma separated list of response headers to pass from remote server")
var cache tieredCache
var signatureKeys signatureKeyList
var scaleUp = flag.Bool("scaleUp", false, "allow images to scale beyond their original dimensions")
var timeout = flag.Duration("timeout", 0, "time limit for requests served by this proxy")
var verbose = flag.Bool("verbose", false, "print verbose logging messages")
var _ = flag.Bool("version", false, "Deprecated: this flag does nothing")
var contentTypes = flag.String("contentTypes", "image/*", "comma separated list of allowed content types")
var userAgent = flag.String("userAgent", "willnorris/imageproxy", "specify the user-agent used by imageproxy when fetching images from origin website")
var minCacheDuration = flag.Duration("minCacheDuration", 0, "minimum duration to cache remote images")
var forceCache = flag.Bool("forceCache", false, "Ignore no-store and private directives in responses")
var storages prefixStoragesFlag

func init() {
	flag.Var(&cache, "cache", "location to cache images (see https://github.com/willnorris/imageproxy#cache)")
	flag.Var(&signatureKeys, "signatureKey", "HMAC key used in calculating request signatures")
	flag.Var(&storages, "storages", "JSON object mapping the first request path segment to either an origin base URL or an S3 storage definition")
}

func main() {
	envy.Parse("IMAGEPROXY")
	flag.Parse()

	p := imageproxy.NewProxy(nil, cache.Cache)
	if *allowHosts != "" {
		p.AllowHosts = strings.Split(*allowHosts, ",")
	}
	if *denyHosts != "" {
		p.DenyHosts = strings.Split(*denyHosts, ",")
	}
	if *referrers != "" {
		p.Referrers = strings.Split(*referrers, ",")
	}
	if *contentTypes != "" {
		p.ContentTypes = strings.Split(*contentTypes, ",")
	}
	if *passRequestHeaders != "" {
		p.PassRequestHeaders = strings.Split(*passRequestHeaders, ",")
	}
	if *passResponseHeaders != "" {
		p.PassResponseHeaders = strings.Split(*passResponseHeaders, ",")
	} else {
		// set to a non-nil empty slice to pass no headers.
		p.PassResponseHeaders = []string{}
	}
	p.SignatureKeys = signatureKeys
	if *baseURL != "" {
		var err error
		p.DefaultBaseURL, err = url.Parse(*baseURL)
		if err != nil {
			log.Fatalf("error parsing baseURL: %v", err)
		}
	}
	if len(storages) > 0 {
		p.PrefixStorages = map[string]*imageproxy.PrefixStorage(storages)
	}

	p.IncludeReferer = *includeReferer
	p.FollowRedirects = *followRedirects
	p.Timeout = *timeout
	p.ScaleUp = *scaleUp
	p.Verbose = *verbose
	p.UserAgent = *userAgent
	p.MinimumCacheDuration = *minCacheDuration
	p.ForceCache = *forceCache

	var ln net.Listener
	var err error

	if path, ok := strings.CutPrefix(*addr, "unix:"); ok {
		ln, err = net.Listen("unix", path)
	} else {
		ln, err = net.Listen("tcp", *addr)
	}
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	server := &http.Server{
		Addr:    *addr,
		Handler: p,

		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	fmt.Printf("imageproxy listening on %s\n", *addr)
	log.Fatal(server.Serve(ln))
}

type signatureKeyList [][]byte

func (skl *signatureKeyList) String() string {
	return fmt.Sprint(*skl)
}

func (skl *signatureKeyList) Set(value string) error {
	for _, v := range strings.Fields(value) {
		key := []byte(v)
		if strings.HasPrefix(v, "@") {
			file := strings.TrimPrefix(v, "@")
			var err error
			key, err = os.ReadFile(file)
			if err != nil {
				log.Fatalf("error reading signature file: %v", err)
			}
		}
		*skl = append(*skl, key)
	}
	return nil
}

type prefixStoragesFlag map[string]*imageproxy.PrefixStorage

func (pbuf *prefixStoragesFlag) String() string {
	if len(*pbuf) == 0 {
		return ""
	}

	raw := make(map[string]any, len(*pbuf))
	for name, storage := range *pbuf {
		raw[name] = stringifyStorage(storage)
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Sprint(raw)
	}

	return string(data)
}

func (pbuf *prefixStoragesFlag) Set(value string) error {
	parsedStorages, err := parseStorages(value)
	if err != nil {
		return err
	}

	if *pbuf == nil {
		*pbuf = make(prefixStoragesFlag)
	}

	for name, storage := range parsedStorages {
		(*pbuf)[name] = storage
	}

	return nil
}

func parseStorages(value string) (map[string]*imageproxy.PrefixStorage, error) {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, fmt.Errorf("error parsing storages: %w", err)
	}

	storages := make(map[string]*imageproxy.PrefixStorage, len(raw))
	for name, rawStorage := range raw {
		name = strings.TrimSpace(name)
		if err := validateStorageName(name); err != nil {
			return nil, err
		}

		storage, err := parseStorage(rawStorage)
		if err != nil {
			return nil, fmt.Errorf("storage %q: %w", name, err)
		}
		storages[name] = storage
	}

	return storages, nil
}

type rawStorageConfig struct {
	Type             string `json:"type"`
	URL              string `json:"url"`
	BaseURL          string `json:"baseURL"`
	Endpoint         string `json:"endpoint"`
	Region           string `json:"region"`
	Bucket           string `json:"bucket"`
	Prefix           string `json:"prefix"`
	AccessKey        string `json:"accessKey"`
	SecretKey        string `json:"secretKey"`
	SessionToken     string `json:"sessionToken"`
	DisableSSL       bool   `json:"disableSSL"`
	ForcePathStyle   bool   `json:"forcePathStyle"`
	S3ForcePathStyle bool   `json:"s3ForcePathStyle"`
	PresignExpiry    string `json:"presignExpiry"`
}

func parseStorage(raw json.RawMessage) (*imageproxy.PrefixStorage, error) {
	var rawURL string
	if err := json.Unmarshal(raw, &rawURL); err == nil {
		return parseHTTPStorage(rawURL)
	}

	var cfg rawStorageConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid storage config: %w", err)
	}

	switch storageType := strings.ToLower(strings.TrimSpace(cfg.Type)); storageType {
	case "", "http":
		baseURL := strings.TrimSpace(cfg.URL)
		if baseURL == "" {
			baseURL = strings.TrimSpace(cfg.BaseURL)
		}
		if baseURL == "" && cfg.Bucket != "" {
			storageType = "s3"
		} else {
			return parseHTTPStorage(baseURL)
		}
		fallthrough
	case "s3":
		return parseS3Storage(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type %q", cfg.Type)
	}
}

func parseHTTPStorage(rawURL string) (*imageproxy.PrefixStorage, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("must define a base URL")
	}

	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("has invalid base URL: %w", err)
	}
	if !baseURL.IsAbs() {
		return nil, fmt.Errorf("must define an absolute base URL")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("must use http or https")
	}

	return &imageproxy.PrefixStorage{BaseURL: baseURL}, nil
}

func parseS3Storage(cfg rawStorageConfig) (*imageproxy.PrefixStorage, error) {
	var endpoint *url.URL
	if strings.TrimSpace(cfg.Endpoint) != "" {
		parsedEndpoint, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
		if err != nil {
			return nil, fmt.Errorf("has invalid endpoint: %w", err)
		}
		if !parsedEndpoint.IsAbs() {
			return nil, fmt.Errorf("endpoint must be absolute")
		}
		if parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https" {
			return nil, fmt.Errorf("endpoint must use http or https")
		}
		endpoint = parsedEndpoint
	}

	storage := &imageproxy.PrefixStorage{
		S3: &imageproxy.S3Storage{
			Endpoint:       endpoint,
			Region:         strings.TrimSpace(cfg.Region),
			Bucket:         strings.TrimSpace(cfg.Bucket),
			Prefix:         strings.Trim(strings.TrimSpace(cfg.Prefix), "/"),
			AccessKey:      strings.TrimSpace(cfg.AccessKey),
			SecretKey:      strings.TrimSpace(cfg.SecretKey),
			SessionToken:   strings.TrimSpace(cfg.SessionToken),
			DisableSSL:     cfg.DisableSSL,
			ForcePathStyle: cfg.ForcePathStyle || cfg.S3ForcePathStyle,
		},
	}

	if strings.TrimSpace(cfg.PresignExpiry) != "" {
		expiry, err := time.ParseDuration(strings.TrimSpace(cfg.PresignExpiry))
		if err != nil {
			return nil, fmt.Errorf("has invalid presignExpiry: %w", err)
		}
		storage.S3.PresignExpiry = expiry
	}

	if err := storage.S3.Validate(); err != nil {
		return nil, err
	}

	return storage, nil
}

func validateStorageName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("storage name cannot be empty")
	case strings.Contains(name, "/"):
		return fmt.Errorf("storage name %q cannot contain /", name)
	}

	return nil
}

func stringifyStorage(storage *imageproxy.PrefixStorage) any {
	if storage == nil {
		return nil
	}
	if storage.BaseURL != nil {
		return storage.BaseURL.String()
	}
	if storage.S3 == nil {
		return nil
	}

	raw := map[string]any{
		"type":      "s3",
		"bucket":    storage.S3.Bucket,
		"accessKey": storage.S3.AccessKey,
		"secretKey": storage.S3.SecretKey,
	}
	if storage.S3.Endpoint != nil {
		raw["endpoint"] = storage.S3.Endpoint.String()
	}
	if storage.S3.Region != "" {
		raw["region"] = storage.S3.Region
	}
	if storage.S3.Prefix != "" {
		raw["prefix"] = storage.S3.Prefix
	}
	if storage.S3.SessionToken != "" {
		raw["sessionToken"] = storage.S3.SessionToken
	}
	if storage.S3.DisableSSL {
		raw["disableSSL"] = true
	}
	if storage.S3.ForcePathStyle {
		raw["forcePathStyle"] = true
	}
	if storage.S3.PresignExpiry > 0 {
		raw["presignExpiry"] = storage.S3.PresignExpiry.String()
	}
	return raw
}

// tieredCache allows specifying multiple caches via flags, which will create
// tiered caches using the twotier package.
type tieredCache struct {
	imageproxy.Cache
}

func (tc *tieredCache) String() string {
	return fmt.Sprint(*tc)
}

func (tc *tieredCache) Set(value string) error {
	for _, v := range strings.Fields(value) {
		c, err := parseCache(v)
		if err != nil {
			return err
		}

		if tc.Cache == nil {
			tc.Cache = c
		} else {
			tc.Cache = twotier.New(tc.Cache, c)
		}
	}
	return nil
}

// parseCache parses c returns the specified Cache implementation.
func parseCache(c string) (imageproxy.Cache, error) {
	if c == "" {
		return nil, nil
	}

	if c == "memory" {
		c = fmt.Sprintf("memory:%d", defaultMemorySize)
	}

	u, err := url.Parse(c)
	if err != nil {
		return nil, fmt.Errorf("error parsing cache flag: %w", err)
	}

	switch u.Scheme {
	case "azure":
		return azurestoragecache.New("", "", u.Host)
	case "gcs":
		return gcscache.New(u.Host, strings.TrimPrefix(u.Path, "/"))
	case "memory":
		return lruCache(u.Opaque)
	case "redis":
		conn, err := redis.DialURL(u.String(), redis.DialPassword(os.Getenv("REDIS_PASSWORD")))
		if err != nil {
			return nil, err
		}
		return rediscache.NewWithClient(conn), nil
	case "s3":
		return s3cache.New(u.String())
	case "file":
		return diskCache(u.Path), nil
	default:
		return diskCache(c), nil
	}
}

// lruCache creates an LRU Cache with the specified options of the form
// "maxSize:maxAge".  maxSize is specified in megabytes, maxAge is a duration.
func lruCache(options string) (*lrucache.LruCache, error) {
	parts := strings.SplitN(options, ":", 2)
	size, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}

	var age time.Duration
	if len(parts) > 1 {
		age, err = time.ParseDuration(parts[1])
		if err != nil {
			return nil, err
		}
	}

	return lrucache.New(size*1e6, int64(age.Seconds())), nil
}

func diskCache(path string) *diskcache.Cache {
	d := diskv.New(diskv.Options{
		BasePath: path,

		// For file "c0ffee", store file as "c0/ff/c0ffee"
		Transform: func(s string) []string { return []string{s[0:2], s[2:4]} },
	})
	return diskcache.NewWithDiskv(d)
}
