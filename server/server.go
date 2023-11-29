package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

var worldCopy [][]uint8
var turn int

var sync bool
var turnHundred int

var paused bool

var reset bool

var clients []*rpc.Client

var clientsReady bool

func createWorldCopy(world [][]uint8) [][]uint8 {
	worldCopy := make([][]uint8, len(world))
	for i := range worldCopy {
		worldCopy[i] = make([]uint8, len(world[i]))
		copy(worldCopy[i], world[i])
	}
	return worldCopy
}

func calculateAliveCells(world [][]uint8, height int, width int) []util.Cell {
	var newCell []util.Cell
	for j := 0; j < height; j++ {
		for i := 0; i < width; i++ {
			if world[j][i] == 255 {
				addedCell := util.Cell{
					X: i,
					Y: j,
				}
				newCell = append(newCell, addedCell)
			}
		}
	}

	return newCell
}

func numberOfAliveCells(world [][]uint8, height, width int) int {
	aliveCells := calculateAliveCells(world, height, width)
	sum := 0
	for range aliveCells {
		sum++
	}
	return sum
}

func createWorldCutCopy(world [][]uint8, startY, endY int) [][]uint8 {
	numRows := endY - startY + 1
	fmt.Println("EndY:", endY, "StartY:", startY, "numRows:", numRows)
	worldCut := make([][]uint8, numRows)
	for i := 0; i < numRows; i++ {
		// Calculate the index in the original slice
		originalIndex := startY + i

		// Copy the row to the subset
		worldCut[i] = make([]uint8, len(world[originalIndex]))
		copy(worldCut[i], world[originalIndex])
	}
	fmt.Println("Height of worldCut:", len(worldCut), "Width of worldCut:", len(worldCut[0]))
	return worldCut
}

func checkChunkPosition(Begin, End, height int, world [][]uint8) [][]uint8 {
	var worldCopy [][]uint8
	if Begin == 0 && End == height-1 {
		worldCopy = append(createWorldCutCopy(world, height-1, height-1), append(createWorldCutCopy(world, 0, height-1), createWorldCutCopy(world, 0, 0)...)...)
	} else if Begin == 0 {
		worldCopy = append(createWorldCutCopy(world, height-1, height-1), createWorldCutCopy(world, Begin, End+1)...)
	} else if End == height-1 {
		worldCopy = append(createWorldCutCopy(world, Begin-1, End), createWorldCutCopy(world, 0, 0)...)
	} else {
		worldCopy = createWorldCutCopy(world, Begin-1, End+1)
	}
	return worldCopy
}

//Removed c DistributerChannels, p gol.Params as they were only needs for SDL
//Removed threads arg as it was only needed for parallel
func remoteDistributor(world [][]uint8, turns int, threads int) [][]uint8 {

	//fmt.Println("-------------------------------------Remote Distributor Called------------------------------")

	turnHundred = 0
	threads = 2
	turn = 0
	worldCopy = createWorldCopy(world)
	height := len(world)

	chunkSize := height / threads
	remainingChunk := height % threads

	var bufferedSliceChan = make([]chan [][]uint8, threads)

	clients = make([]*rpc.Client, threads)

	errs := make([]error, threads)
	/*
		address := make([]string, 8)
		address[0] = "54.242.253.12:8040"
		address[1] = "34.229.159.250:8040"
		address[2] = "52.23.230.155:8040"
		address[3] = "54.162.208.37:8040"
		address[4] = "34.224.78.220:8040"
		address[5] = "54.226.16.66:8040"
		address[6] = "34.227.195.170:8040"
		address[7] = "50.19.31.194:8040"
	*/
	for i := 0; i < threads; i++ {

		port := 8040 + (i * 10)
		address := "localhost:" + fmt.Sprint(port)
		fmt.Println(address)
		clients[i], errs[i] = rpc.Dial("tcp", address)
		if errs[i] != nil {
			fmt.Println("-----------Unable to connect--------------------")
		}
	}

	clientsReady = true
	//fmt.Println("NUMBER OF TURNS:", turns)

	fmt.Println("--------------------------------Turn:", turn, "------------------------------------------------")

	//Timer sends time down channel to notify SDL of the number of alive cells and turns completed every 2 seconds
	//timer := time.NewTicker(2 * time.Second)

	//Execute all turns of the Game of Life and Populate Alive cells.
	//if threads == 1 {
	//fmt.Println("---------------------------ONE THREAD----------------------------------------")
	//for i := 0; i < turns; i++ {
	//fmt.Println("FOR LOOP ENTERED")
	//fmt.Println("--------------------------------Turn:", turn, "------------------------------------------------")

	if reset == true {
		reset = false
		return world
	}

	//world = parallelCalculateNextState(worldCopy, 0, height, height, width)
	//fmt.Println("STATES ABOUT TO BE CALCULATED")
	for k := 0; k < threads; k++ {
		fmt.Println("K=", k, " Threads=", threads)
		if k < threads-remainingChunk {
			Begin := k * chunkSize
			End := (k + 1) * chunkSize
			fmt.Println("Begin: ", Begin, " End: ", End)
			fmt.Println("len(worldCopy[0]):", len(worldCopy[0]))
			fmt.Println("len(worldCopy):", len(worldCopy))
			bufferedSliceChan[k] = make(chan [][]uint8, 1)
			toSend := checkChunkPosition(Begin, End-1, height, worldCopy)
			request := stubs.WorkerRequest{
				WorldCopy: toSend,
				StartY:    1,
				EndY:      len(toSend) - 1,
				Turns:     turns,
			}
			response := new(stubs.WorkerResponse)
			go func(k int, request stubs.WorkerRequest, response *stubs.WorkerResponse, channel chan [][]uint8) {
				err := clients[k].Call(stubs.WorkerCalculate, request, response)
				if err != nil {
					fmt.Println("Error calling Worker Calculate")
				}
				bufferedSliceChan[k] <- response.World
			}(k, request, response, bufferedSliceChan[k])

		} else if k == threads-remainingChunk {
			Begin := k * chunkSize
			End := (k+1)*chunkSize + 1
			bufferedSliceChan[k] = make(chan [][]uint8, 1)
			toSend := checkChunkPosition(Begin, End-1, height, worldCopy)
			request := stubs.WorkerRequest{
				WorldCopy: toSend,
				StartY:    1,
				EndY:      len(toSend) - 1,
				Turns:     turns,
			}
			response := new(stubs.WorkerResponse)
			go func(k int, request stubs.WorkerRequest, response *stubs.WorkerResponse, channel chan [][]uint8) {
				err := clients[k].Call(stubs.WorkerCalculate, request, response)
				if err != nil {
					fmt.Println("Error calling Worker Calculate")
				}
				bufferedSliceChan[k] <- response.World
			}(k, request, response, bufferedSliceChan[k])

		} else if k > threads-remainingChunk {
			Begin := (k * chunkSize) + (k - (threads - remainingChunk))
			End := (k+1)*chunkSize + (k + 1 - (threads - remainingChunk))
			bufferedSliceChan[k] = make(chan [][]uint8, 1)
			toSend := checkChunkPosition(Begin, End-1, height, worldCopy)
			request := stubs.WorkerRequest{
				WorldCopy: toSend,
				StartY:    1,
				EndY:      len(toSend) - 1,
				Turns:     turns,
			}
			response := new(stubs.WorkerResponse)
			go func(k int, request stubs.WorkerRequest, response *stubs.WorkerResponse, channel chan [][]uint8) {
				err := clients[k].Call(stubs.WorkerCalculate, request, response)
				if err != nil {
					fmt.Println("Error calling Worker Calculate")
				}
				bufferedSliceChan[k] <- response.World
			}(k, request, response, bufferedSliceChan[k])

		}
	}

	fmt.Println("STATES CALCULATED")

	resetChannel := make(chan bool, 1)
	terminate := make(chan bool, 1)
	go func(resetChannel chan bool, terminate chan bool) {
		for {
			select {
			case <-terminate:
				return
			default:
				if reset == true {
					resetChannel <- true
				}
			}
		}
	}(resetChannel, terminate)

	var parallelWorld [][]uint8

	for i := 0; i < threads; i++ {
		select {
		case s := <-bufferedSliceChan[i]:
			parallelWorld = append(parallelWorld, s...)
		case <-resetChannel:
			return world
		}

	}
	terminate <- true
	fmt.Println("DONE")

	//fmt.Println("TURN ADVANCED")

	//sync prevents the cell count being read while the turn and cell count are out of sync
	sync = false
	world = parallelWorld
	worldCopy = createWorldCopy(world)
	turn++
	/*
		if turn%100 == 0 {
			turnHundred++
			//fmt.Println("--------------------HUNDRED TURNS-------------------------------")
		}
	*/
	sync = true

	//}

	return world
}

type RemoteProcessor struct{}

func (r *RemoteProcessor) CallNumberOfAliveCells(request stubs.CellCountRequest, response *stubs.CellCountResponse) (err error) {
	/*
		done := false

		for done != true {
			if sync == true {
				response.Turn = turn
				if turn == 0 {
					response.CellCount = 0
				} else {
					response.CellCount = numberOfAliveCells(worldCopy, len(worldCopy), len(worldCopy[0]))
				}
				done = true
			}
		}
	*/

	for !clientsReady {
	}

	match := 0
	cellCount := 0
	turn1 := 0

	for match != len(clients) {
		//fmt.Println("NUMBEROFALIVECELLSCALLED-----------------------", len(clients))

		cellCount = 0
		match = 0
		turn1 = 0
		requests := make([]stubs.CellCountRequest, len(clients))
		responses := make([]*stubs.CellCountResponse, len(clients))
		done := make(chan bool, len(clients))
		for i, _ := range clients {
			//fmt.Println("Iterating through clients")
			requests[i] = stubs.CellCountRequest{}
			responses[i] = new(stubs.CellCountResponse)

			go func(i int, responses []*stubs.CellCountResponse, requests []stubs.CellCountRequest, done chan bool) {
				err := clients[i].Call(stubs.WorkerCellCount, requests[i], responses[i])
				if err != nil {
					fmt.Println("Unable to call cell count of client ", i)
				}
				done <- true
				//fmt.Println("done pushed")

			}(i, responses, requests, done)
		}

		for i := 0; i < len(clients); i++ {

			<-done
			//fmt.Println("NUMBEROFALIVECELLSCALLED-----------------------", i, " ", len(clients))

		}
		for i, v := range responses {
			if i == 0 {
				turn1 = v.Turn
			}
			cellCount += v.CellCount
			if turn1 == v.Turn {
				match++
			}

		}
		//fmt.Println("From CellCount, match:", match)
	}

	response.CellCount = cellCount
	response.Turn = turn1

	//fmt.Printf("Reported CellCount: %d, Reported turn: %d\n", response.CellCount, response.Turn)
	return
}

func (r *RemoteProcessor) CallPause(request stubs.PauseReq, response *stubs.PauseResp) (err error) {
	match := 0
	turn1 := 0

	for match != len(clients) {
		//fmt.Println("NUMBEROFALIVECELLSCALLED-----------------------", len(clients))

		match = 0
		turn1 = 0
		requests := make([]stubs.PauseReq, len(clients))
		responses := make([]*stubs.PauseResp, len(clients))
		done := make(chan bool, len(clients))
		for i, _ := range clients {
			//fmt.Println("Iterating through clients")
			requests[i] = stubs.PauseReq{Paused: request.Paused}
			responses[i] = new(stubs.PauseResp)

			go func(i int, responses []*stubs.PauseResp, requests []stubs.PauseReq, done chan bool) {
				err := clients[i].Call(stubs.WorkerPause, requests[i], responses[i])
				if err != nil {
					fmt.Println("Unable to call pause of client ", i)
				}
				done <- true
				//fmt.Println("done pushed")

			}(i, responses, requests, done)
		}

		for i := 0; i < len(clients); i++ {

			<-done
			//fmt.Println("NUMBEROFALIVECELLSCALLED-----------------------", i, " ", len(clients))

		}
		for i, v := range responses {
			if i == 0 {
				turn1 = v.Turn
			}

			if turn1 == v.Turn {
				match++
			}

		}
		//fmt.Println("From CellCount, match:", match)
	}

	response.Turn = turn1

	//fmt.Printf("Reported CellCount: %d, Reported turn: %d\n", response.CellCount, response.Turn)
	return
	/*paused = request.Paused
	response.Turn = turn
	*/

}

func (r *RemoteProcessor) CallSave(request stubs.SaveReq, response *stubs.SaveResp) (err error) {
	match := 0
	turn1 := 0
	var parallelWorld [][]uint8

	for match != len(clients) {
		//fmt.Println("Savecalled-----------------------", len(clients))
		match = 0
		turn1 = 0
		parallelWorld = nil
		requests := make([]stubs.SaveReq, len(clients))
		responses := make([]*stubs.SaveResp, len(clients))
		done := make(chan bool, len(clients))
		for i, _ := range clients {
			//fmt.Println("Iterating through clients")
			requests[i] = stubs.SaveReq{}
			responses[i] = new(stubs.SaveResp)

			go func(i int, responses []*stubs.SaveResp, requests []stubs.SaveReq, done chan bool) {
				err := clients[i].Call(stubs.WorkerSave, requests[i], responses[i])
				if err != nil {
					fmt.Println("Unable to call save of worker ", i)
				}
				done <- true
				//fmt.Println("done pushed")

			}(i, responses, requests, done)
		}

		for i := 0; i < len(clients); i++ {

			<-done
			//fmt.Println("NUMBEROFclientscompleted----------------------", i, " ", len(clients))

		}
		for i, v := range responses {
			if i == 0 {
				turn1 = v.Turn
			}

			if turn1 == v.Turn {
				match++
			}

			//fmt.Println("Response: ", i, " ", len(v.World), "x", len(v.World[0]))
			parallelWorld = append(parallelWorld, v.World...)

		}
		//fmt.Println("From save, match:", match)
	}

	response.Turn = turn1
	response.World = parallelWorld
	fmt.Println("Save:", len(parallelWorld), "x", len(parallelWorld[0]))

	//fmt.Printf("Reported CellCount: %d, Reported turn: %d\n", response.CellCount, response.Turn)
	return
	/*
		response.World = worldCopy
		response.Turn = turn
		return

	*/
}

func (r *RemoteProcessor) CallClose(request stubs.CloseReq, response *stubs.CloseResp) (err error) {
	reset = true
	time.Sleep(1 * time.Second)
	for i, v := range clients {
		err := v.Call(stubs.WorkerClose, stubs.CloseReq{}, new(stubs.CloseResp))
		if err != nil {
			//fmt.Println(err)
		}
		err = v.Close()
		if err != nil {
			fmt.Println("Couldn't close rpc connection to worker number", i)
		}
	}
	os.Exit(0)
	return
}

func (r *RemoteProcessor) CallRemoteDistributor(request stubs.Request, response *stubs.Response) (err error) {
	fmt.Println("-------------------------------------RPC For Remote Distributor Called------------------------------")
	reset = true
	clientsReady = false

	time.Sleep(1 * time.Second)
	reset = false
	//world := request.World //testing purposes only so i dont have to edit the test loop below
	response.World = remoteDistributor(request.World, request.Turns, request.Threads)
	//fmt.Println("Request:", len(request.World), "x", len(request.World[0]), "Response:", len(response.World), "x", len(response.World[0]))
	fmt.Println("DISTRIBUTOR COMPLETE")

	//fmt.Println(turn)
	/*
		test := 0
		for i, _ := range world {
			for i2, _ := range world[i] {
				if world[i][i2] == response.World[i][i2] {
					test++
				}
			}
		}
		if test == len(world)*len(world[0]) {
			//fmt.Println("-----------------------FUCK-------------------------")
		}
	*/

	return
}

/*
func connectToWorkers(threads int, clients []*rpc.Client) {
	clients = make([]*rpc.Client, threads)
	errs := make([]error, threads)
	for i := 0; i < threads; i++ {

		port := 8040 + (i * 10)
		address := "localhost:" + fmt.Sprint(port)
		fmt.Println(address)
		clients[i], errs[i] = rpc.Dial("tcp", address)
		if errs[i] != nil {
			fmt.Println("-----------Unable to connect--------------------")
		}

		defer func(client *rpc.Client) {
			err := client.Close()
			if err != nil {
				fmt.Println("-----------Unable to close connection--------------------")
			}
		}(clients[i])

	}
}
*/
func main() {
	pAddr := flag.String("port", ":8030", "Port to listen on")
	flag.Parse()

	reset = false
	paused = false
	clientsReady = false

	listener, _ := net.Listen("tcp", *pAddr)
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			fmt.Println("Error closing the listener")
		}
	}(listener)
	err := rpc.Register(&RemoteProcessor{})
	if err != nil {
		fmt.Println("Error registering rpc")
	}

	rpc.Accept(listener)

}
