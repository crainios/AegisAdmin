package apache

import (
	"context"
	"sync"
)

type overviewResult struct {
	data map[string]any
	err  *Error
}

func (b *LinuxBackend) overview(ctx context.Context) (map[string]any, *Error) {
	var waitGroup sync.WaitGroup
	results := make(map[string]overviewResult, 5)
	var resultsMu sync.Mutex

	run := func(name string, operation func() (map[string]any, *Error)) {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			data, err := operation()
			resultsMu.Lock()
			results[name] = overviewResult{data: data, err: err}
			resultsMu.Unlock()
		}()
	}

	run("info", func() (map[string]any, *Error) {
		return b.info(ctx)
	})
	run("configtest", func() (map[string]any, *Error) {
		valid, message, err := b.configTest(ctx)
		return map[string]any{"valid": valid, "message": message}, err
	})
	run("vhosts", func() (map[string]any, *Error) {
		return b.vhosts(ctx)
	})
	run("sites", func() (map[string]any, *Error) {
		return b.sites()
	})
	run("modules", func() (map[string]any, *Error) {
		return b.modules(ctx)
	})

	waitGroup.Wait()

	data := make(map[string]any, len(results))
	for _, name := range []string{"info", "configtest", "vhosts", "sites", "modules"} {
		result := results[name]
		if result.err != nil {
			return nil, result.err
		}
		data[name] = result.data
	}
	return data, nil
}
