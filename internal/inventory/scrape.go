// Package inventory retrieves the player's Warframe inventory the same way
// AlecaFrame does: it scrapes the game-server auth tokens (accountId + nonce)
// from the running Warframe process's memory, then calls Digital Extremes'
// official mobile inventory API. This needs no Overwolf, but it is unsanctioned
// use of DE's API and requires permission to read the game's memory.
//
// Reading /proc/<pid>/mem of the (same-user) game process requires either
// kernel.yama.ptrace_scope=0 or the CAP_SYS_PTRACE capability on this binary;
// otherwise the read fails with EACCES (surfaced as ErrPermission).
package inventory

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ErrNotRunning is returned when no Warframe process can be found.
var ErrNotRunning = errors.New("inventory: Warframe process not found")

// ErrPermission is returned when the game's memory cannot be read due to
// ptrace restrictions.
var ErrPermission = errors.New("inventory: cannot read game memory (need kernel.yama.ptrace_scope=0 or CAP_SYS_PTRACE)")

// ErrAuthNotFound is returned when the auth pattern is not present in memory
// (e.g. the player is not yet logged in to a game session).
var ErrAuthNotFound = errors.New("inventory: accountId/nonce not found in game memory (are you logged in?)")

// Auth holds the scraped game-server credentials used to query the inventory API.
type Auth struct {
	AccountID string // 24-char hex account id
	Nonce     string // numeric session nonce
}

// Query returns the URL query string ("?accountId=...&nonce=...").
func (a Auth) Query() string {
	return "?accountId=" + a.AccountID + "&nonce=" + a.Nonce
}

const (
	// Process comm is truncated to 15 chars by the kernel.
	procName      = "Warframe.x64.exe"
	procNameTrunc = "Warframe.x64.ex"
)

// FindWarframePID locates the running Warframe process, returning ErrNotRunning
// if absent.
func FindWarframePID() (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if name == procName || name == procNameTrunc {
			return pid, nil
		}
	}
	return 0, ErrNotRunning
}

// authPattern is the literal that precedes the account id in the game's auth
// query string ("?accountId=").
var authPattern = []byte("?accountId=")

const (
	accountIDLen = 24
	noncePrefix  = "&nonce="
)

// ScrapeAuth scans the given process's readable memory for the auth tokens. It
// collects all candidates and returns the most frequently seen one (stale copies
// of an old nonce may linger in memory).
func ScrapeAuth(pid int) (Auth, error) {
	regions, err := readableRegions(pid)
	if err != nil {
		return Auth{}, err
	}
	mem, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return Auth{}, ErrPermission
		}
		return Auth{}, err
	}
	defer mem.Close()

	votes := map[Auth]int{}
	sawAny := false
	permDenied := false

	const chunk = 4 << 20 // 4 MiB read window
	const overlap = 128   // enough to hold accountId + nonce after the pattern
	buf := make([]byte, chunk+overlap)

	for _, r := range regions {
		for off := r.start; off < r.end; off += chunk {
			n := chunk + overlap
			if off+uint64(n) > r.end {
				n = int(r.end - off)
			}
			read, err := mem.ReadAt(buf[:n], int64(off))
			if err != nil && read == 0 {
				if errors.Is(err, os.ErrPermission) {
					permDenied = true
				}
				break // region unreadable; move on
			}
			scanChunk(buf[:read], &votes, &sawAny)
		}
	}

	if len(votes) == 0 {
		if permDenied {
			return Auth{}, ErrPermission
		}
		return Auth{}, ErrAuthNotFound
	}

	// Pick the most-voted candidate.
	var best Auth
	bestVotes := -1
	for a, v := range votes {
		if v > bestVotes {
			bestVotes, best = v, a
		}
	}
	return best, nil
}

// scanChunk finds every occurrence of the auth pattern in data and records the
// extracted Auth.
func scanChunk(data []byte, votes *map[Auth]int, sawAny *bool) {
	from := 0
	for {
		idx := indexOf(data[from:], authPattern)
		if idx < 0 {
			return
		}
		pos := from + idx
		from = pos + 1
		a, ok := extractAuth(data[pos:])
		if ok {
			*sawAny = true
			(*votes)[a]++
		}
	}
}

// extractAuth parses "?accountId=<24>&nonce=<digits>" starting at the pattern.
func extractAuth(b []byte) (Auth, bool) {
	p := len(authPattern)
	if len(b) < p+accountIDLen+len(noncePrefix)+1 {
		return Auth{}, false
	}
	id := b[p : p+accountIDLen]
	if !isHexID(id) {
		return Auth{}, false
	}
	rest := b[p+accountIDLen:]
	if !hasPrefixBytes(rest, noncePrefix) {
		return Auth{}, false
	}
	rest = rest[len(noncePrefix):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return Auth{}, false
	}
	return Auth{AccountID: string(id), Nonce: string(rest[:end])}, true
}

// region is a half-open memory range [start, end).
type region struct{ start, end uint64 }

// readableRegions parses /proc/<pid>/maps for readable, writable, anonymous
// regions (the heap, where the auth string lives). Falling back to all readable
// regions would be slower and is unnecessary in practice.
func readableRegions(pid int) ([]region, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil, ErrPermission
		}
		return nil, err
	}
	defer f.Close()

	var regions []region
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		perms := fields[1]
		if len(perms) < 2 || perms[0] != 'r' || perms[1] != 'w' {
			continue
		}
		// Skip file-backed mappings (a pathname in field 6); the auth string is
		// in anonymous heap memory.
		if len(fields) >= 6 && fields[5] != "" {
			continue
		}
		dash := strings.IndexByte(fields[0], '-')
		if dash < 0 {
			continue
		}
		start, err1 := strconv.ParseUint(fields[0][:dash], 16, 64)
		end, err2 := strconv.ParseUint(fields[0][dash+1:], 16, 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		regions = append(regions, region{start, end})
	}
	return regions, sc.Err()
}

func isHexID(b []byte) bool {
	for _, c := range b {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func hasPrefixBytes(b []byte, s string) bool {
	if len(b) < len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

// indexOf is a small Boyer-Moore-Horspool-free substring search (patterns are
// short and rare, so the naive scan is fine and avoids allocations).
func indexOf(haystack, needle []byte) int {
	n, m := len(haystack), len(needle)
	if m == 0 || m > n {
		return -1
	}
	first := needle[0]
	for i := 0; i <= n-m; i++ {
		if haystack[i] != first {
			continue
		}
		match := true
		for j := 1; j < m; j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
