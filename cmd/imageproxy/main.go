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
var storages prefixBaseURLsFlag

func init() {
	flag.Var(&cache, "cache", "location to cache images (see https://github.com/willnorris/imageproxy#cache)")
	flag.Var(&signatureKeys, "signatureKey", "HMAC key used in calculating request signatures")
	flag.Var(&storages, "storages", "JSON object mapping the first request path segment to a base URL")
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
		p.PrefixBaseURLs = map[string]*url.URL(storages)
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

type prefixBaseURLsFlag map[string]*url.URL

func (pbuf *prefixBaseURLsFlag) String() string {
	if len(*pbuf) == 0 {
		return ""
	}

	raw := make(map[string]string, len(*pbuf))
	for name, baseURL := range *pbuf {
		raw[name] = baseURL.String()
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Sprint(raw)
	}

	return string(data)
}

func (pbuf *prefixBaseURLsFlag) Set(value string) error {
	baseURLs, err := parseStorages(value)
	if err != nil {
		return err
	}

	if *pbuf == nil {
		*pbuf = make(prefixBaseURLsFlag)
	}

	for name, baseURL := range baseURLs {
		(*pbuf)[name] = baseURL
	}

	return nil
}

func parseStorages(value string) (map[string]*url.URL, error) {
	raw := map[string]string{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, fmt.Errorf("error parsing storages: %w", err)
	}

	baseURLs := make(map[string]*url.URL, len(raw))
	for name, rawURL := range raw {
		name = strings.TrimSpace(name)
		rawURL = strings.TrimSpace(rawURL)

		switch {
		case name == "":
			return nil, fmt.Errorf("storage name cannot be empty")
		case strings.Contains(name, "/"):
			return nil, fmt.Errorf("storage name %q cannot contain /", name)
		case rawURL == "":
			return nil, fmt.Errorf("storage %q must define a base URL", name)
		}

		baseURL, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("storage %q has invalid base URL: %w", name, err)
		}
		if !baseURL.IsAbs() {
			return nil, fmt.Errorf("storage %q must define an absolute base URL", name)
		}
		if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
			return nil, fmt.Errorf("storage %q must use http or https", name)
		}

		baseURLs[name] = baseURL
	}

	return baseURLs, nil
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
