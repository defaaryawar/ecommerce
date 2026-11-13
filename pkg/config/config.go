// Package config: helper kecil buat baca konfigurasi dari environment variable.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// String mengembalikan nilai env, atau def kalau kosong.
func String(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// MustString mengembalikan nilai env, panic kalau kosong.
func MustString(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: env %s wajib diisi", key))
	}
	return v
}

// Int membaca env sebagai integer.
func Int(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("config: env %s harus angka: %v", key, err))
	}
	return n
}

// Bool membaca env sebagai boolean (true/false/1/0).
func Bool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		panic(fmt.Sprintf("config: env %s harus boolean: %v", key, err))
	}
	return b
}

// Duration membaca env sebagai durasi (contoh: 15m, 1h).
func Duration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("config: env %s harus durasi (contoh 15m): %v", key, err))
	}
	return d
}
