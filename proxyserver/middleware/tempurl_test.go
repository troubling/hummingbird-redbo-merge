//  Copyright (c) 2017 Rackspace
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
//  implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package middleware

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDispositionFormat(t *testing.T) {
	require.Equal(t, "inline; filename=\"a.txt\"; filename*=UTF-8''a.txt", dispositionFormat("inline", "a.txt"))
	require.Equal(t, "attachment; filename=\"%25.txt\"; filename*=UTF-8''%25.txt", dispositionFormat("attachment", "%.txt"))
}

func TestParseExpires(t *testing.T) {
	d, err := parseExpires("1493708668")
	require.Nil(t, err)
	require.EqualValues(t, 1493708668, d.Unix())

	d, err = parseExpires("2017-05-02T07:04:28Z")
	require.Nil(t, err)
	require.EqualValues(t, 1493708668, d.Unix())

	d, err = parseExpires("FAIL")
	require.NotNil(t, err)
}

func TestCheckHmac(t *testing.T) {
	// test cases generated by example python code
	sig, err := hex.DecodeString("6deb0c7da21f396f1368681dc0bd57df0d1c4369")
	require.Nil(t, err)
	require.True(t, checkhmac([]byte("mykey"), sig, "GET",
		"/v1/AUTH_account/container/object", time.Unix(1493709631, 0).In(time.UTC)))

	// sig is actually for a POST, but make sure we can HEAD with it.
	sig, err = hex.DecodeString("1ad2301fcc4e525ee0167298c0fbb426e90fb3b1")
	require.Nil(t, err)
	require.True(t, checkhmac([]byte("mykey"), sig, "HEAD",
		"/v1/AUTH_account/container/object", time.Unix(1493709631, 0).In(time.UTC)))

	// sig is actually for a POST, but make sure we can HEAD with it.
	sig, err = hex.DecodeString("1111111111111111111111111111111111111111")
	require.Nil(t, err)
	require.False(t, checkhmac([]byte("mykey"), sig, "HEAD",
		"/v1/AUTH_account/container/object", time.Unix(1493709631, 0).In(time.UTC)))
}

func TestTuWriter(t *testing.T) {
	w := &tuWriter{ResponseWriter: httptest.NewRecorder(), method: "GET", obj: "a.txt",
		filename: "", expires: "whatever", inline: true}
	w.Header().Set("X-Object-Meta-Test", "XXX")
	w.Header().Set("X-Object-Meta-Public-Test", "ZZZ")
	w.WriteHeader(200)
	require.Equal(t, "", w.Header().Get("X-Object-Meta-Test"))
	require.Equal(t, "ZZZ", w.Header().Get("X-Object-Meta-Public-Test"))
	require.Equal(t, "inline", w.Header().Get("Content-Disposition"))

	w = &tuWriter{ResponseWriter: httptest.NewRecorder(), method: "GET", obj: "a.txt",
		filename: "b.txt", expires: "whatever", inline: true}
	w.WriteHeader(200)
	require.Equal(t, "inline; filename=\"b.txt\"; filename*=UTF-8''b.txt", w.Header().Get("Content-Disposition"))

	w = &tuWriter{ResponseWriter: httptest.NewRecorder(), method: "GET", obj: "a.txt",
		filename: "b.txt", expires: "whatever", inline: false}
	w.WriteHeader(200)
	require.Equal(t, "attachment; filename=\"b.txt\"; filename*=UTF-8''b.txt", w.Header().Get("Content-Disposition"))

	w = &tuWriter{ResponseWriter: httptest.NewRecorder(), method: "GET", obj: "a.txt",
		filename: "", expires: "whatever", inline: false}
	w.WriteHeader(200)
	require.Equal(t, "attachment; filename=\"a.txt\"; filename*=UTF-8''a.txt", w.Header().Get("Content-Disposition"))
}

func TestTempurlMiddlewarePassOptions(t *testing.T) {
	r := httptest.NewRequest("OPTIONS", "/v1/something", nil)
	w := httptest.NewRecorder()
	served := false
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, w, writer)
		require.Equal(t, r, request)
		served = true
	})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.True(t, served)
}

func TestTempurlMiddlewarePassAlreadyAuthorized(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/something", nil)
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext",
		&ProxyContext{
			Authorize: func(r *http.Request) bool {
				return false
			},
		}))
	w := httptest.NewRecorder()
	served := false
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, w, writer)
		require.Equal(t, r, request)
		served = true
	})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.True(t, served)
}

func TestTempurlMiddlewarePassNoQuery(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/something", nil)
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", &ProxyContext{}))
	w := httptest.NewRecorder()
	served := false
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, w, writer)
		require.Equal(t, r, request)
		served = true
	})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.True(t, served)
}

func TestTempurlMiddleware401OnlySig(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/something?temp_url_sig=ABCDEF", nil)
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", &ProxyContext{}))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 401, w.Result().StatusCode)
}

func TestTempurlMiddleware401Expired(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/something?temp_url_sig=ABCDEF&temp_url_expires=0", nil)
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", &ProxyContext{}))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 401, w.Result().StatusCode)
}

func TestTempurlMiddleware401BadSig(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/something?temp_url_sig=ABCDEFXXX&temp_url_expires=9999999999", nil)
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", &ProxyContext{}))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 401, w.Result().StatusCode)
}

func TestTempurlMiddleware401NoContainer(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/something?temp_url_sig=ABCDEF&temp_url_expires=9999999999", nil)
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", &ProxyContext{}))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 401, w.Result().StatusCode)
}

func TestTempurlMiddleware400PuttingManifest(t *testing.T) {
	r := httptest.NewRequest("PUT", "/v1/a/c/o?temp_url_sig=ABCDEF&temp_url_expires=9999999999", nil)
	r.Header.Set("X-Object-Manifest", "true")
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", &ProxyContext{}))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 400, w.Result().StatusCode)
}

func TestTempurlMiddleware401NoKeys(t *testing.T) {
	r := httptest.NewRequest("PUT", "/v1/a/c/o?temp_url_sig=ABCDEF&temp_url_expires=9999999999", nil)
	ctx := &ProxyContext{
		containerInfoCache: map[string]*containerInfo{
			"container/a/c": {metadata: map[string]string{}},
		},
		accountInfoCache: map[string]*AccountInfo{
			"account/a": {Metadata: map[string]string{}},
		},
	}
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", ctx))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 401, w.Result().StatusCode)
}

func TestTempurlMiddleware401WrongKeys(t *testing.T) {
	r := httptest.NewRequest("PUT", "/v1/a/c/o?temp_url_sig=ABCDEF&temp_url_expires=9999999999", nil)
	ctx := &ProxyContext{
		containerInfoCache: map[string]*containerInfo{
			"container/a/c": {metadata: map[string]string{"Temp-Url-Key": "ABCD", "Temp-Url-Key-2": "012345"}},
		},
		accountInfoCache: map[string]*AccountInfo{
			"account/a": {Metadata: map[string]string{"Temp-Url-Key": "ABCD", "Temp-Url-Key-2": "012345"}},
		},
	}
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", ctx))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 401, w.Result().StatusCode)
}

func TestTempurlMiddlewareContainerKey(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/a/c/o?temp_url_sig=f2d61be897a27c03ac9a0dac3a8c4f6ce3a3d623&"+
		"temp_url_expires=9999999999", nil)
	ctx := &ProxyContext{
		containerInfoCache: map[string]*containerInfo{
			"container/a/c": {metadata: map[string]string{"Temp-Url-Key": "mykey"}},
		},
		accountInfoCache: map[string]*AccountInfo{"account/a": {Metadata: map[string]string{}}},
	}
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", ctx))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := GetProxyContext(request)
		require.NotNil(t, ctx.Authorize)
		require.True(t, ctx.Authorize(request))
		// make sure authorizing requests to other containers fail, since we're using a container key.
		require.False(t, ctx.Authorize(httptest.NewRequest("GET", "/v1/a/b/o", nil)))
		// but other objects in the container are fine?
		require.True(t, ctx.Authorize(httptest.NewRequest("GET", "/v1/a/c/o2", nil)))
		writer.WriteHeader(200)
	})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 200, w.Result().StatusCode)
}

func TestTempurlMiddlewarePath(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/a/c/o123?temp_url_sig=058e0771c69f7e1eb1eacbd68396920fd06ff261&"+
		"temp_url_expires=9999999999&temp_url_prefix=o", nil)
	ctx := &ProxyContext{
		containerInfoCache: map[string]*containerInfo{
			"container/a/c": {metadata: map[string]string{"Temp-Url-Key": "mykey"}},
		},
		accountInfoCache: map[string]*AccountInfo{"account/a": {Metadata: map[string]string{}}},
	}
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", ctx))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := GetProxyContext(request)
		require.NotNil(t, ctx.Authorize)
		require.True(t, ctx.Authorize(request))
		writer.WriteHeader(200)
	})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 200, w.Result().StatusCode)
}

func TestTempurlMiddlewareAccountKey(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/a/c/o?temp_url_sig=f2d61be897a27c03ac9a0dac3a8c4f6ce3a3d623&"+
		"temp_url_expires=9999999999", nil)
	ctx := &ProxyContext{
		containerInfoCache: map[string]*containerInfo{
			"container/a/c": {metadata: map[string]string{}},
		},
		accountInfoCache: map[string]*AccountInfo{
			"account/a": {Metadata: map[string]string{"Temp-Url-Key": "mykey"}}},
	}
	r = r.WithContext(context.WithValue(r.Context(), "proxycontext", ctx))
	w := httptest.NewRecorder()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := GetProxyContext(request)
		require.NotNil(t, ctx.Authorize)
		require.True(t, ctx.Authorize(request))
		// check that authorizing with other containers in the account works
		require.True(t, ctx.Authorize(httptest.NewRequest("GET", "/v1/a/b/o", nil)))
		// but not other accounts, obvs.
		require.False(t, ctx.Authorize(httptest.NewRequest("GET", "/v1/a2/b/o", nil)))
		writer.WriteHeader(200)
	})
	mid := tempurl(handler)
	mid.ServeHTTP(w, r)
	require.Equal(t, 200, w.Result().StatusCode)
}
