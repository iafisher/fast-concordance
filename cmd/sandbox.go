package main

import (
	"log"
	"sync"
	"time"
)

func writeToUnbufferedChannelBlocksUntilRead() {
	// go1: write
	// go2: sleep
	// go2: read

	var wg sync.WaitGroup

	ch := make(chan int)

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("write: before (should block)")
		ch <- 42
		log.Println("write: after")
	}()

	time.Sleep(1 * time.Second)

	log.Println("read: before (shouldn't block)")
	x := <-ch
	log.Printf("read: after (got %d)", x)

	close(ch)
	wg.Wait()
}

func readFromUnbufferedChannelBlocksUntilWritten() {
	// go1: read
	// go2: sleep
	// go2: write

	var wg sync.WaitGroup

	ch := make(chan int)

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("read: before (should block)")
		x := <-ch
		log.Printf("read: after (got: %d)", x)
	}()

	time.Sleep(1 * time.Second)

	log.Println("write: before (shouldn't block)")
	ch <- 1
	log.Println("write: after")

	close(ch)
	wg.Wait()
}

func writeToBufferedChannelBlocksIfFull() {
	// go1: write
	// go1: write
	// go2: sleep
	// go2: read

	var wg sync.WaitGroup

	ch := make(chan int, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("write1: before (shouldn't block)")
		ch <- 42
		log.Println("write: after")

		log.Println("write1: before (should block)")
		ch <- 43
		log.Println("write1: after")
	}()

	time.Sleep(1 * time.Second)

	log.Println("read: before (shouldn't block)")
	x := <-ch
	log.Printf("read: after (got %d)", x)

	close(ch)
	wg.Wait()
}

func closingChannelTwicePanics() {
	ch := make(chan int)
	close(ch)
	close(ch)
}

func readingFromClosedChannelReturnsDefault() {
	ch := make(chan int)
	close(ch)
	x := <-ch
	log.Printf("got: %d", x)

	_, ok := <-ch
	log.Printf("ok: %v", ok)
}

func readingFromBufferedClosedChannelReturnsBufferedValue() {
	ch := make(chan int, 1)
	ch <- 42
	close(ch)
	x := <-ch
	log.Printf("got: %d", x)
}

func writingToClosedChannelPanics() {
	ch := make(chan int)
	close(ch)
	ch <- 42
}

func closedChannelCanBeCheckedMultipleTimes() {
	ch := make(chan struct{})
	close(ch)

	select {
	case <-ch:
		log.Println("channel closed (1)")
	default:
		log.Println("default (1)")
	}

	select {
	case <-ch:
		log.Println("channel closed (2)")
	default:
		log.Println("default (2)")
	}
}

func selectCanCheckIfWriteWillBlock() {
	ch := make(chan int)

	select {
	case ch <- 42:
		log.Println("wrote to channel")
	default:
		log.Println("write would have blocked")
	}
}

func selectWithoutAssignmentRemovesValueFromChannel() {
	ch := make(chan int, 2)
	ch <- 42
	ch <- 43

	select {
	case <-ch:
		log.Println("value on channel")
	default:
		log.Println("no value on channel")
	}

	x := <-ch
	log.Printf("got: %d", x)
}

func selectCasesAreRandom() {
	buffer := 10
	ch1 := make(chan int, buffer)
	ch2 := make(chan int, buffer)

	// buffer needs to be as large as number of iterations in case we get unlucky and
	// always read from the same channel; in that case we don't want the other channel's
	// buffer to get full and then block, causing a deadlock
	for i := 0; i < buffer; i++ {
		ch1 <- 42
		ch2 <- 63

		select {
		case x := <-ch1:
			log.Printf("got %d from ch1", x)
		case x := <-ch2:
			log.Printf("got %d from ch2", x)
		}
	}
}

func runtimeWillPanicIfGoroutineDeadlocksItself() {
	ch := make(chan int)
	x := <-ch
	log.Printf("got: %d", x)
}

func selectCasesDoNotFallthrough() {
	ch1 := make(chan int, 1)
	ch2 := make(chan int, 1)

	ch1 <- 42

	select {
	case <-ch1:
	case <-ch2:
		log.Println("should not see this!")
	}
}

func selectWithoutDefaultBlocks() {
	ch := make(chan struct{})

	go func() {
		time.Sleep(1 * time.Second)
		close(ch)
	}()

	log.Printf("read: before (should block)")
	select {
	case <-ch:
	}
	log.Printf("read: after")
}

func selectWithDefaultDoesNotBlock() {
	ch := make(chan struct{})

	log.Printf("read: before (shouldn't block)")
	select {
	case <-ch:
	default:
	}
	log.Printf("read: after")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// readFromUnbufferedChannelBlocksUntilWritten()
	// writeToUnbufferedChannelBlocksUntilRead()
	// writeToBufferedChannelBlocksIfFull()
	// closingChannelTwicePanics()
	// readingFromClosedChannelReturnsDefault()
	// readingFromBufferedClosedChannelReturnsBufferedValue()
	// writingToClosedChannelPanics()
	// closedChannelCanBeCheckedMultipleTimes()
	// selectCanCheckIfWriteWillBlock()
	// selectWithoutAssignmentRemovesValueFromChannel()
	// selectCasesAreRandom()
	// runtimeWillPanicIfGoroutineDeadlocksItself()
	// selectCasesDoNotFallthrough()
	// selectWithoutDefaultBlocks()
	selectWithDefaultDoesNotBlock()
}
