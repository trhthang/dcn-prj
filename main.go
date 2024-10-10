package main

import "fmt"

// func printNumbers() {
// 	for i := 1; i <= 5; i++ {
// 		fmt.Println(i)
// 		time.Sleep(100 * time.Millisecond)
// 	}
// }

// func main() {
// 	go printNumbers()

// 	time.Sleep(600 * time.Millisecond)
// 	fmt.Println("Main finished")
// }

// func sendData(ch chan int) {
// 	for i := 0; i < 10; i++ {
// 		ch <- i
// 	}
// 	close(ch)
// }

// func main() {
// 	ch := make(chan int)

// 	go sendData(ch) // chay goroutine

// 	for received := range ch {
// 		fmt.Println("Received: ", received)
// 	}
// }

func main() {
	ch := make(chan int, 3)

	ch <- 1
	ch <- 2
	ch <- 3

	fmt.Println(<-ch)
	fmt.Println(<-ch)
	fmt.Println(<-ch)
}
