package main

import "encoding/json"

func jsonImpl(data []byte, v any) error { return json.Unmarshal(data, v) }
