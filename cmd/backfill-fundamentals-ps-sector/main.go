package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	path := flag.String("file", "data/fundamentals.json", "fundamentals.json path")
