package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type SearchResponse struct {
	Kind              string `json:"kind"`
	SearchInformation struct {
		SearchTime   float64 `json:"searchTime"`
		TotalResults string  `json:"totalResults"`
	} `json:"searchInformation"`
	Items []SearchItem `json:"items"`
}

type SearchItem struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	DisplayLink string `json:"displayLink"`
	Snippet     string `json:"snippet"`
	PageMap     struct {
		Product []struct {
			Name string `json:"name"`
		} `json:"product"`
		AggregateOffer []struct {
			PriceCurrency string `json:"pricecurrency"`
			LowPrice      string `json:"lowprice"`
			HighPrice     string `json:"highprice"`
		} `json:"aggregateoffer"`
	} `json:"pagemap"`
}

type CartItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	Price       string `json:"price"`
	Shop        string `json:"shop"`
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
}

type Cart struct {
	Items map[string]*CartItem
	mutex sync.RWMutex
}

var cart = &Cart{
	Items: make(map[string]*CartItem),
}

type Config struct {
	GoogleAPIKey   string
	SearchEngineID string
}

func loadConfig() *Config {
	return &Config{
		GoogleAPIKey:   os.Getenv("GOOGLE_API_KEY"),
		SearchEngineID: os.Getenv("GOOGLE_SEARCH_ENGINE_ID"),
	}
}

func searchProducts(query string, numResults int) (*SearchResponse, error) {
	config := loadConfig()
	if config.GoogleAPIKey == "" || config.SearchEngineID == "" {
		return nil, fmt.Errorf("Google API key or Search Engine ID not configured")
	}

	baseURL := "https://www.googleapis.com/customsearch/v1"
	params := url.Values{}
	params.Add("key", config.GoogleAPIKey)
	params.Add("cx", config.SearchEngineID)
	params.Add("q", query)
	params.Add("num", strconv.Itoa(numResults))

	resp, err := http.Get(baseURL + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("failed to make search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResponse SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	return &searchResponse, nil
}

func addToCart(itemID, title, link, price, shop, description string) {
	cart.mutex.Lock()
	defer cart.mutex.Unlock()

	if existingItem, exists := cart.Items[itemID]; exists {
		existingItem.Quantity++
	} else {
		cart.Items[itemID] = &CartItem{
			ID:          itemID,
			Title:       title,
			Link:        link,
			Price:       price,
			Shop:        shop,
			Description: description,
			Quantity:    1,
		}
	}
}

func removeFromCart(itemID string) bool {
	cart.mutex.Lock()
	defer cart.mutex.Unlock()

	if item, exists := cart.Items[itemID]; exists {
		if item.Quantity > 1 {
			item.Quantity--
		} else {
			delete(cart.Items, itemID)
		}
		return true
	}
	return false
}

func getCart() map[string]*CartItem {
	cart.mutex.RLock()
	defer cart.mutex.RUnlock()

	result := make(map[string]*CartItem)
	for k, v := range cart.Items {
		result[k] = &CartItem{
			ID:          v.ID,
			Title:       v.Title,
			Link:        v.Link,
			Price:       v.Price,
			Shop:        v.Shop,
			Description: v.Description,
			Quantity:    v.Quantity,
		}
	}
	return result
}

func clearCart() {
	cart.mutex.Lock()
	defer cart.mutex.Unlock()
	cart.Items = make(map[string]*CartItem)
}

type queryParams struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type numResultsParams struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     int    `json:"default"`
}

func main() {
	s := server.NewMCPServer(
		"shopping-server",
		"1.0.0",
		server.WithLogging(),
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
	)

	s.AddTool(mcp.Tool{
		Name:        "search_products",
		Description: "Поиск товаров по запросу с использованием Google Custom Search API",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"query": queryParams{
					Type:        "string",
					Description: "Поисковый запрос для поиска товаров",
				},
				"num_results": numResultsParams{
					Type:        "integer",
					Description: "Количество результатов поиска (по умолчанию 10, максимум 10)",
					Default:     10,
				},
			},
			Required: []string{"query"},
		},
	}, handleSearchProducts)

	s.AddTool(mcp.Tool{
		Name:        "view_cart",
		Description: "Посмотреть содержимое корзины",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
		},
	}, handleViewCart)

	// fmt.Println("GOOGLE_API_KEY =", os.Getenv("GOOGLE_API_KEY"))
	// fmt.Println("SEARCHENGINEID =", os.Getenv("GOOGLE_SEARCH_ENGINE_ID"))

	// serverCert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	// if err != nil {
	// 	log.Fatalf("failed to load server key pair: %v", err)
	// }

	// clientCACert, err := ioutil.ReadFile("ca.crt")
	// if err != nil {
	// 	log.Fatalf("failed to read client CA cert: %v", err)
	// }
	// clientCertPool := x509.NewCertPool()
	// clientCertPool.AppendCertsFromPEM(clientCACert)

	// tlsConfig := &tls.Config{
	// 	Certificates: []tls.Certificate{serverCert},
	// 	ClientAuth:   tls.RequireAndVerifyClientCert,
	// 	ClientCAs:    clientCertPool,
	// 	MinVersion:   tls.VersionTLS12,
	// }

	// serverHTTP := &http.Server{
	// 	Addr:      ":8443",
	// 	TLSConfig: tlsConfig,
	// 	Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 		if len(r.TLS.PeerCertificates) > 0 {
	// 			clientCert := r.TLS.PeerCertificates[0]
	// 			fmt.Fprintf(w, "Hello, %s!\n", clientCert.Subject.CommonName)
	// 		} else {
	// 			http.Error(w, "No client certificate provided", http.StatusUnauthorized)
	// 		}
	// 	}),
	// }

	// httpServer := server.NewStreamableHTTPServer(s, server.WithStreamableHTTPServer(serverHTTP))
	httpServer := server.NewStreamableHTTPServer(s)
	if err := httpServer.Start("localhost:8080"); err != nil {
		log.Fatal(err)
	}
}

func handleSearchProducts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Invalid arguments format"},
			},
		}, nil
	}

	query, ok := args["query"].(string)
	if !ok {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "query parameter is required and must be a string"},
			},
		}, nil
	}

	numResults := 10
	if num, ok := args["num_results"].(float64); ok {
		numResults = int(num)
		if numResults > 10 {
			numResults = 10
		}
	}

	searchResponse, err := searchProducts(query, numResults)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: fmt.Sprintf("Search failed: %v", err)},
			},
		}, nil
	}

	var results []string
	for i, item := range searchResponse.Items {
		price := "Цена не указана"
		if len(item.PageMap.AggregateOffer) > 0 {
			offer := item.PageMap.AggregateOffer[0]
			if offer.LowPrice != "" {
				price = fmt.Sprintf("от %s %s", offer.LowPrice, offer.PriceCurrency)
			}
		}

		result := fmt.Sprintf(`📦 Товар #%d
🏷️ Название: %s
🏪 Магазин: %s
💰 Цена: %s
🔗 Ссылка: %s
📝 Описание: %s
🆔 ID для корзины: %s
---`,
			i+1,
			item.Title,
			item.DisplayLink,
			price,
			item.Link,
			item.Snippet,
			generateItemID(item),
		)
		results = append(results, result)
	}

	totalResults := searchResponse.SearchInformation.TotalResults
	searchTime := searchResponse.SearchInformation.SearchTime

	finalResult := fmt.Sprintf(`🔍 Результаты поиска для "%s"
📊 Найдено: %s результатов за %.2f секунд
📋 Показаны первые %d результатов:

%s

💡 Используйте add_to_cart с ID товара для добавления в корзину`,
		query, totalResults, searchTime, len(results), strings.Join(results, "\n"))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: finalResult},
		},
	}, nil
}

func handleViewCart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cartItems := getCart()

	if len(cartItems) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "🛒 Корзина пуста"},
			},
		}, nil
	}

	var items []string
	totalItems := 0
	for _, item := range cartItems {
		totalItems += item.Quantity
		itemText := fmt.Sprintf(`📦 %s
🏪 Магазин: %s
💰 Цена: %s
🔢 Количество: %d
🔗 Ссылка: %s
🆔 ID: %s
---`,
			item.Title,
			item.Shop,
			item.Price,
			item.Quantity,
			item.Link,
			item.ID)
		items = append(items, itemText)
	}

	result := fmt.Sprintf(`🛒 Ваша корзина
📊 Всего товаров: %d (уникальных: %d)

%s

💡 Используйте remove_from_cart с ID для удаления товара`,
		totalItems, len(cartItems), strings.Join(items, "\n"))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: result},
		},
	}, nil
}

func generateItemID(item SearchItem) string {
	return fmt.Sprintf("%s-%s", item.DisplayLink, strings.ReplaceAll(item.Link, "/", "-"))
}
