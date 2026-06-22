// Command wfo-inventory tests the inventory module: it finds the running
// Warframe process, scrapes the accountId/nonce from its memory, and optionally
// fetches the inventory JSON.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"warframe-overlay-linux/internal/inventory"
)

func main() {
	fetch := flag.Bool("fetch", false, "fetch inventory JSON from DE and write it to -o")
	out := flag.String("o", "/tmp/inventory.json", "where to write fetched inventory JSON")
	from := flag.String("from", "", "parse a local inventory JSON file and print owned counts for args")
	flag.Parse()

	if *from != "" {
		raw, err := os.ReadFile(*from)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read:", err)
			os.Exit(1)
		}
		inv, err := inventory.Parse(raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "parse:", err)
			os.Exit(1)
		}
		fmt.Printf("parsed %d owned prime parts\n", inv.Len())
		for _, c := range inv.Categories() {
			fmt.Printf("  %-18s %d\n", c.Name, len(c.Items))
		}
		for _, name := range flag.Args() {
			fmt.Printf("  owned %-30s = %d\n", name, inv.Owned(name))
		}
		return
	}

	pid, err := inventory.FindWarframePID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "find process:", err)
		os.Exit(1)
	}
	fmt.Println("Warframe PID:", pid)

	auth, err := inventory.ScrapeAuth(pid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scrape auth:", err)
		os.Exit(1)
	}
	fmt.Printf("accountId: %s…%s  nonce: %s\n",
		auth.AccountID[:6], auth.AccountID[len(auth.AccountID)-4:], auth.Nonce)

	if !*fetch {
		fmt.Println("(pass -fetch to download the inventory)")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	body, err := inventory.FetchRaw(ctx, auth)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, body, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d bytes to %s\n", len(body), *out)
}
