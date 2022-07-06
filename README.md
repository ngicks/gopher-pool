# gopher-pool [![Godoc](https://godoc.org/github.com/ngicks/gopher-pool?status.svg)](https://godoc.org/github.com/ngicks/gopher-pool)

Yet another Worker Pool implementation.

Gophers are swimming!

## Example

```go
package main

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	gopherpool "github.com/ngicks/gopher-pool"
)

func main() {
	workCh := make(chan gopherpool.WorkFn)
	pool := gopherpool.NewWorkerPool(
		gopherpool.SetDefaultWorkerConstructor(
			workCh,
			nil,
			nil,
		),
	)

	wg := sync.WaitGroup{}
	wg.Add(100)
	go func() {
		for i := 0; i < 100; i++ {
			num := i
			workCh <- func(ctx context.Context) {
				fmt.Printf("%d,", num)
				time.Sleep(time.Second)
				wg.Done()
			}
		}
	}()

	go func() {
		for {
			fmt.Printf("\ncurrent active worker: %d\n", pool.ActiveWorkerNum())
			time.Sleep(500 * time.Millisecond)
		}
	}()

	fmt.Printf("current goroutine num: %d\n", runtime.NumGoroutine())
	pool.Add(32)
	fmt.Println("added 32 workers")
	fmt.Printf("current goroutine num: %d\n", runtime.NumGoroutine())

	time.Sleep(500 * time.Millisecond)

	pool.Remove(16)
	fmt.Println("removed 16 workers")
	fmt.Printf("current goroutine num: %d\n", runtime.NumGoroutine())

	wg.Wait()

	time.Sleep(time.Second)
	pool.Remove(math.MaxUint32)
	pool.Wait()
}

```

outputs:

```
current goroutine num: 3

current active worker: 0
added 32 workers
current goroutine num: 35
0,17,16,1,9,13,10,15,12,11,3,4,2,7,6,8,18,19,20,21,22,14,5,31,30,26,27,23,24,25,29,28,
current active worker: 32
removed 16 workers
current goroutine num: 35
32,33,
current active worker: 31
34,36,35,47,41,37,38,39,40,44,42,43,45,46,
current active worker: 16
48,49,50,57,55,52,59,60,51,61,54,53,58,56,63,62,
current active worker: 16

current active worker: 16
64,65,66,67,73,68,78,70,71,72,69,79,74,75,77,76,
current active worker: 16

current active worker: 16
80,81,82,83,84,89,86,90,85,87,88,95,91,92,93,94,
current active worker: 16

current active worker: 16
96,97,98,99,
current active worker: 4

current active worker: 4

current active worker: 0

current active worker: 0
```
