package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	url          = "https://eth.getblock.io/mainnet/"
	apiKey       = "effce8eb-20b7-457f-8efc-fcbfb8814f11"
	zeroValueHex = "0x0"
)

type lastBlockResp struct {
	Id      string `json:"id"`
	Jsonrpc string `json:"jsonrpc"`
	Result  string `json:"result"`
}

type blockByNumberResp struct {
	Id      string `json:"id"`
	Jsonrpc string `json:"jsonrpc"`
	Result  block  `json:"result"`
}

type block struct {
	Hash         string        `json:"hash"`
	Number       string        `json:"number"`
	Transactions []transaction `json:"transactions"`
}

type transaction struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"`
}

func main() {
	var latestBlockNumber string

	latestBlockNumber, err := getLatestBlockNumber()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	latestBlockNumberInt := int(convertHexToBig(latestBlockNumber).Int64())

	// создаем мапу для хранения баланса каждого адреса
	balances := make(map[string]*big.Int)
	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}

	//пришлось сделать тикер, чтобы не ловить 429
	rateLimiter := time.Tick(time.Millisecond * 25)

	// запрос eth_getBlockByNumber для последних 100 блоков
	for i := latestBlockNumberInt; i > latestBlockNumberInt-100; i-- {
		<-rateLimiter
		wg.Add(1)
		go func(num int) {
			curBlock, err := getBlock(num)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				panic(err)
			}

			for _, tx := range curBlock.Transactions {
				if tx.Value == zeroValueHex {
					continue
				}
				// изменяем баланс отправителя
				mu.Lock()
				if _, ok := balances[tx.From]; !ok {
					balances[tx.From] = big.NewInt(0)
				}

				numVal := convertHexToBig(tx.Value)
				balances[tx.From].Sub(balances[tx.From], numVal)

				mu.Unlock()

				// изменяем баланс получателя
				mu.Lock()
				if _, ok := balances[tx.To]; !ok {
					balances[tx.To] = big.NewInt(0)
				}
				balances[tx.To].Add(balances[tx.To], convertHexToBig(tx.Value))

				mu.Unlock()
			}
			wg.Done()
		}(i)
	}

	wg.Wait()

	// находим адрес с наибольшим изменением баланса
	var maxAddress string
	var maxBalanceChange *big.Int

	for address, balance := range balances {
		if maxBalanceChange == nil || balance.Cmp(maxBalanceChange) != 0 {
			maxAddress = address
			maxBalanceChange = balance
		}
	}

	// выводим адрес с наибольшим изменением баланса
	fmt.Printf("Address: %v\nBalance Change: %v", maxAddress, maxBalanceChange.Int64())
}

func getBlock(i int) (block, error) {
	blockNumberHex := fmt.Sprintf("%#x", i)

	payload := strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["%s",true],"id":"getblock.io"}`, blockNumberHex))

	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return block{}, err
	}

	req.Header.Add("x-api-key", apiKey)
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return block{}, err
	}

	defer func() {
		if res != nil {
			err = res.Body.Close()
			if err != nil {
				fmt.Println(err)
			}
		}
	}()

	var result blockByNumberResp

	if res.StatusCode != http.StatusOK {
		return block{}, errors.New("request failed")
	}

	if err = json.NewDecoder(res.Body).Decode(&result); err != nil {
		return block{}, err
	}

	return result.Result, nil
}

func getLatestBlockNumber() (string, error) {
	payload := strings.NewReader(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":"getblock.io"}`)
	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return "", err
	}

	req.Header.Add("x-api-key", apiKey)
	req.Header.Add("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	defer res.Body.Close()

	var resp lastBlockResp
	if err = json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return "", err
	}

	return resp.Result, err
}

// конвертация hex в decimal
func convertHexToBig(hex string) *big.Int {
	num := new(big.Int)
	num, _ = num.SetString(hex[2:], 16)

	return num
}
