package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell, sendChans []chan uint8, receiveChans []chan uint8, turnCh []chan int,  keyChan <-chan rune) {
	ticker := time.NewTicker(2 * time.Second)
	world := make([][]byte, p.imageHeight)
	for i := range world {
		world[i] = make([]byte, p.imageWidth)
	}

	// Request the io goroutine to read in the image with the given filename.
	d.io.command <- ioInput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")

	// The io goroutine sends the requested image byte by byte, in rows.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			val := <-d.io.inputVal
			if val != 0 {
				fmt.Println("Alive cell at", x, y)
				world[y][x] = val
			}

		}
	}

	for turns := 0; turns < p.turns; turns++ {

		select {
		case val := <-keyChan:
			switch val {
			case 's':
				// update world from worker at the moment
				// receive to world
				d.io.command <- ioOutput
				d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight), strconv.Itoa(turns)}, "x")
				for y := 0; y < p.imageHeight; y++ {
					for x := 0; x < p.imageWidth; x++ {
						d.io.outputVal <- world[y][x]
					}
				}
				// send back to worker
				// world to send
			case 'q':
				//update world
				// receive to world
				d.io.command <- ioOutput
				d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight), strconv.Itoa(turns)}, "x")
				// distributor to io
				for y := 0; y < p.imageHeight; y++ {
					for x := 0; x < p.imageWidth; x++ {
						d.io.outputVal <- world[y][x]
					}
				}
				d.io.command <- ioCheckIdle
				<-d.io.idle
				fmt.Println("Quitting...")
				StopControlServer()
				os.Exit(0)
			case 'p':
				fmt.Println("Current turn:", turns)
				for 'p' != <-keyChan {
					// paused
				}
				fmt.Println("Continuing")

			}

		default:
			select {
			case <-ticker.C:
				//fmt.Println("tick")

				fmt.Printf("Num of alive cells : %d\n", aliveCells(p,world))
				// world to send

			default:
				// sync turn
				for i :=range turnCh{
					turnCh[i]<-turns
				}

				if turns == 0 {
					//world to send
					for y := 0; y < p.imageHeight; y++ {
						workerHeight := p.imageHeight/p.threads
						worker := y / workerHeight
						if y % workerHeight == 0 {
							top := y-1
							if top == -1 {
								top = p.imageHeight-1
							}
							for x := 0; x < p.imageWidth; x++ {
								sendChans[worker] <- world[top][x]

							}
						}

						for x := 0; x < p.imageWidth; x++ {
							sendChans[worker] <-world[y][x]

						}

						if y % workerHeight == workerHeight-1 {
							bottom := y+1
							if bottom == p.imageHeight {
								bottom = 0
							}
							for x := 0; x < p.imageWidth; x++ {
								sendChans[worker] <- world[bottom][x]

							}
						}

					}
				}

				if turns == p.turns-1{
					// get updated wold
					receiving(p,receiveChans,world)
				}

			}

		}

	}


	// for the test
	// Create an empty slice to store coordinates of cells that are still alive after p.turns are done.
	var finalAlive []cell
	// Go through the world and append the cells that are still alive.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y][x] != 0 {
				finalAlive = append(finalAlive, cell{x: x, y: y})
			}
		}
	}

	d.io.command <- ioOutput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			d.io.outputVal <- world[y][x]
		}
	}

	// Make sure that the Io has finished any output before exiting.
	d.io.command <- ioCheckIdle
	<-d.io.idle

	// Return the coordinates of cells that are still alive.
	alive <- finalAlive
}

func worker(p golParams, sendChan chan uint8, receiveChan chan uint8, topChan chan uint8, bottomChan chan uint8, topChans []chan uint8, bottomChans []chan uint8,turnCh chan int,  index int) {
	wHeight := (p.imageHeight / p.threads) + 2
	width := p.imageWidth

	worker := make([][]byte, wHeight)
	for i := range worker {
		worker[i] = make([]byte, width)
	}

	aliveNs := make([][]int, wHeight)
	for i := range aliveNs {
		aliveNs[i] = make([]int, width)
	}

	for {
		turn := <-turnCh

		//at first, make worker with halo
		if turn == 0 {

			for y := 0; y < wHeight; y++ {
				for x := 0; x < width; x++ {
					worker[y][x] = <-sendChan
				}
			}
		}

		// Calculate neighbours
		for y := 1; y < wHeight-1; y++ {
			for x := 0; x < width; x++ {
				aliveN := 0
				for i := -1; i <= 1; i++ {
					for j := -1; j <= 1; j++ {
						h := y + i
						w := x + j

						if w == -1 {
							w = width - 1
						} else if w == width { //No need to check Y value due to halo
							w = 0
						}

						if worker[h][w] == 0xFF {
							aliveN++
						}
					}
				}
				if worker[y][x] == 0xFF {
					aliveN--
				}
				aliveNs[y][x] = aliveN

			}
		}

		//flipping
		for y := 1; y < wHeight-1; y++ { // without halo
			for x := 0; x < width; x++ {
				if (worker[y][x] == 0xFF) && (aliveNs[y][x] < 2) {
					worker[y][x] = worker[y][x] ^ 0xFF
				} else if (worker[y][x] == 0xFF) && (aliveNs[y][x] > 3) {
					worker[y][x] = worker[y][x] ^ 0xFF
				} else if (worker[y][x] == 0x00) && (aliveNs[y][x] == 3) {
					worker[y][x] = worker[y][x] ^ 0xFF
				}
				if worker[y][x] == 0xFF {

				}
			}
		}


		// updating halos
		for x := 0; x < width; x++ {
			//send back to workers
			bottomChans[inbound(index-1, p.threads)] <- worker[1][x]
			topChans[inbound(index+1, p.threads)] <- worker[wHeight-2][x]
			worker[0][x] = <-topChan
			worker[wHeight-1][x] = <- bottomChan
		}

		if turn == p.turns -1 {
			workerToReceive(p,receiveChan,worker)
		}

	}


}

func aliveCells(p golParams, world [][]byte) int{
	alivecell :=0
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y][x] != 0 {
				alivecell +=1
			}
		}
	}
	return alivecell
}

func inbound(y int, bound int) int{

	if y <0 {
		y = bound-1
	}else if y == bound{
		y=0
	}
	return y

}

func receiving(p golParams, receiveChans []chan uint8, world [][]uint8){
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			workerHeight := p.imageHeight/p.threads
			quo := y / workerHeight
			world[y][x] = <-receiveChans[quo]
		}
	}

}

func workerToReceive(p golParams,receiveChan chan uint8, worker [][]uint8){
	wHeight := (p.imageHeight / p.threads) + 2
	width := p.imageWidth

	for y := 1; y < wHeight-1; y++ {
		for x := 0; x < width; x++ {
			receiveChan <- worker[y][x]
		}
	}
}