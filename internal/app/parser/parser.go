package parser

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/UshakovN/practice/internal/app/common"
	"github.com/UshakovN/practice/internal/app/store"
	json "github.com/buger/jsonparser"
)

type Parser struct {
	Brand Brand
}

type Brand struct {
	Name string
	Code string
}

func NewParser(brand Brand) *Parser {
	return &Parser{
		Brand: Brand{
			Name: brand.Name,
			Code: brand.Code,
		},
	}
}

func (parser *Parser) getPagesCount(doc *goquery.Document) (int, error) {
	pages, exist := doc.Find("div.node-pagination").
		Find("li").Last().Prev().Find("a").Attr("href")
	// doc.Find("li.hide_in_mobile").Last().Find("a").Attr("href")
	if !exist {
		return 0, errors.New("not found pages")
	}
	return strconv.Atoi(strings.Trim(strings.TrimSpace(pages), `?page=`))
}

func (parser *Parser) getItemsUrl(doc *goquery.Document) ([]string, error) {
	buffer := make([]string, 0)
	doc.Find("div.search_results>div.search_results_listing>div.row.search_result_item").Each(
		func(index int, item *goquery.Selection) {
			url, exist := item.Find("div.columns>div.block>a").Attr("href")
			if exist {
				buffer = append(buffer, strings.TrimSpace(url))
			}
		})
	if len(buffer) == 0 {
		return nil, errors.New("not found urls") // DEBUG
	}
	return buffer, nil
}

func (parser *Parser) getPageDocument(brand Brand, page int) (*goquery.Document, error) {
	url := fmt.Sprintf("https://www.fishersci.com/us/en/brands/%s/%s.html?page=%d",
		brand.Code, brand.Name, page)
	return parser.GetHtmlDocument(url)
}

func (parser *Parser) getItemDocument(item string) (*goquery.Document, error) {
	url := fmt.Sprintf("https://www.fishersci.com%s", item)
	return parser.GetHtmlDocument(url)
}

func (parser *Parser) GetHtmlDocument(url string) (*goquery.Document, error) {
	/*
		zyte := proxy.NewZyteProxy("zyte-proxy-ca.cer")
		client := &http.Client{
			Transport: zyte.GetHttpTransport(),
		}
	*/
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error status code: %d %s", resp.StatusCode, resp.Status)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func getCurrentTimeUTC() string {
	return time.Now().UTC().String()
}

func (parser *Parser) getItemPriceFromAPI(itemArtc string) (float64, error) {
	url := "https://www.fishersci.com/shop/products/service/pricing"
	resp, err := http.PostForm(url, neturl.Values{"partNumber": {itemArtc}})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	priceStr, err := json.GetString(body, "priceAndAvailability", itemArtc, "[0]", "price")
	if err != nil {
		return 0, err
	}
	price, err := strconv.ParseFloat(strings.ReplaceAll(priceStr, `$`, ""), 64)
	if err != nil {
		return 0, err
	}
	/*
		priceStruct := Price{}
		err = json.Unmarshal(body, &priceStruct)
		if err != nil {
			return "", err
		}
			price := priceStruct.PriceAndAvailability.PartNumber[0].Price
			if price == "" {
				return "", errors.New("internal error")
			}
	*/
	return price, nil
}

// (secondary)
func (parser *Parser) getSingleItemData(doc *goquery.Document) (*store.ItemData, error) {
	available := true
	selMain := doc.Find("div.singleProductPage")

	reg := regexp.MustCompile(`[[:^ascii:]]`)

	label := strings.ReplaceAll(
		reg.ReplaceAllLiteralString(
			strings.TrimSpace(
				selMain.Find("div.productSelectors>h1").Contents().First().Text()), " "), "  ", " ")
	if label == "" {
		return nil, errors.New("label not found")
	}

	descript := "None" // secondary items without a description

	// artc, exist := selMain.Find("div.glyphs_html_container").Attr("data-partnumber")
	artc, exist := selMain.Find("input").Last().Attr("data-partnumbers")
	if !exist {
		return nil, errors.New("article not found")
	}
	artc = strings.Split(artc, ",")[0] // temporarily

	manufact := ""
	doc.Find(".spec_table").Find("tr").EachWithBreak(
		func(index int, item *goquery.Selection) bool {
			td := item.Find("td.bold")
			if td.Contents().Text() == "Product Line" {
				manufact = strings.TrimSpace(
					reg.ReplaceAllLiteralString(
						td.Parent().Find("td").Last().Contents().Text(), ""))
				return false
			}
			return true
		})
	if manufact == "" {
		manufact = "None" // temporarily
	}
	manufact = strings.Join([]string{manufact, artc}, " ")

	price, err := parser.getItemPriceFromAPI(artc)
	if err != nil {
		price = 0
		available = false
	}
	/*
		selManAndArt := doc.Find("div.block_head>p")

		manufact := reg.ReplaceAllLiteralString(
			selManAndArt.Last().Contents().Text(), "")

		fmt.Println(doc.Html())

		// not found - idk
		fmt.Println(doc.Find("div.block_head").Nodes)

		if manufact == "" {
			return nil, errors.New("manufacturer not found")
		}

		priceStr := reg.ReplaceAllLiteralString(
			doc.Find("div.block_body>span.qa_single_price>b").Contents().Text(), "")
		if priceStr == "" {
			priceStr = "0"
			available = false
		}

		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			return nil, errors.New("invalid price")
		}

		artc := reg.ReplaceAllLiteralString(
			selManAndArt.First().Contents().Text(), "")
		if artc == "" {
			return nil, errors.New("article not found")
		}
	*/
	created := getCurrentTimeUTC()
	data := &store.ItemData{
		Brand:        strings.Title(parser.Brand.Name),
		Article:      artc,
		Label:        label,
		Description:  descript,
		Manufacturer: manufact,
		Price:        price,
		Available:    available,
		CreatedAt:    created,
	}
	return data, nil
}

func (parser *Parser) getItemData(doc *goquery.Document) (*store.ItemData, []string, error) {
	// in stock
	available := true

	// muiltiple item page
	multipage := doc.Find("div.products_list")
	if multipage.Nodes != nil {
		buffer := make([]string, 0)
		multipage.Find("tbody.itemRowContent").Each(
			func(index int, item *goquery.Selection) {
				url, exist := item.Find("a.chemical_fmly_glyph").Attr("href")
				if exist {
					buffer = append(buffer, url)
				}
			})
		if len(buffer) == 0 {
			return nil, nil, errors.New("not found internal urls")
		}
		return nil, buffer, nil
	}

	// single (secondary) item
	singlepage := doc.Find("div.productSelectors")
	if singlepage.Nodes != nil {
		data, err := parser.getSingleItemData(doc)
		if err != nil {
			return nil, nil, err
		}
		return data, nil, nil
	}

	// default item page
	reg := regexp.MustCompile(`[[:^ascii:]]`)
	selMain := doc.Find("div.product_description_wrapper")

	label := strings.ReplaceAll(
		reg.ReplaceAllLiteralString(
			strings.TrimSpace(
				selMain.Find("h1").Contents().First().Text()), " "), "  ", " ")
	if label == "" {
		return nil, nil, errors.New("label not found")
	}

	selDescAndMan := selMain.Find("div.subhead")

	// default item
	descript := reg.ReplaceAllLiteralString(
		strings.TrimSpace(
			selDescAndMan.Find("p").First().Contents().Text()), "")

	// kit item
	if descript == "" {
		descript = reg.ReplaceAllLiteralString(
			strings.TrimSpace(
				selDescAndMan.Find("div").First().Contents().Text()), "")
	}
	if descript == "" {
		descript = "None" // some items without a description
	}

	// default & kit item
	manufact := strings.ReplaceAll(
		strings.TrimSpace(
			reg.ReplaceAllLiteralString(
				strings.ReplaceAll(
					selDescAndMan.Find("p:nth-of-type(2), p:nth-of-type(3)").
						Contents().Text(), "Manufacturer:", ""), "")), "  ", " ")
	if manufact == "" {
		return nil, nil, errors.New("manufacturer not found")
	}

	selPriceAndArtc := selMain.Find("div.product_sku_options_block")

	priceStr, exist := selPriceAndArtc.Find("label.price>span>span").Attr("content")
	if !exist {
		// not stock
		priceStr = "0"
		available = false
	}
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return nil, nil, errors.New("invalid price")
	}

	artc := strings.TrimSpace(
		selPriceAndArtc.Find("span.float_right").Contents().Text())
	if artc == "" {
		return nil, nil, errors.New("article not found")
	}

	created := getCurrentTimeUTC()
	data := &store.ItemData{
		Brand:        strings.Title(parser.Brand.Name),
		Article:      artc,
		Label:        label,
		Description:  descript,
		Manufacturer: manufact,
		Price:        price,
		Available:    available,
		CreatedAt:    created,
	}
	return data, nil, nil
}

func (parser *Parser) FisherSciencific(client *store.Client) {
	currentPageDoc, err := parser.getPageDocument(parser.Brand, 0)
	if err != nil {
		log.Fatal(err)
	}
	pageCount, err := parser.getPagesCount(currentPageDoc)
	if err != nil {
		log.Fatal(err)
	}
	chanPagesDoc := make(chan *goquery.Document, pageCount)
	defer close(chanPagesDoc)
	go func() {
		// pageCount
		for i := 1; i <= 1; i++ {
			go func(num int) {
				currentPageDoc, err := parser.getPageDocument(parser.Brand, num)
				if err != nil {
					log.Fatal(err)
				}
				chanPagesDoc <- currentPageDoc
			}(i)
		}
	}()
	chanItemsUrl := make(chan []string, pageCount)
	defer close(chanItemsUrl)
	go func() {
		for pagesDoc := range chanPagesDoc {
			go func(doc *goquery.Document) {
				itemsUrl, err := parser.getItemsUrl(doc)
				if err != nil {
					log.Fatal(err)
				}
				chanItemsUrl <- itemsUrl
			}(pagesDoc)
		}
	}()
	chanItemsDoc := make(chan *goquery.Document, 30)
	defer close(chanItemsDoc)
	go func() {
		for itemsUrl := range chanItemsUrl {
			go func(urls []string) {
				for _, currentItemUrl := range urls {
					currentItemDoc, err := parser.getItemDocument(currentItemUrl)
					if err != nil {
						log.Fatal(err)
					}
					chanItemsDoc <- currentItemDoc
				}
			}(itemsUrl)
		}
	}()
	chanItemsData := make(chan *store.ItemData, 30)
	defer close(chanItemsData)
	chanInternalUrls := make(chan []string, 30)
	defer close(chanInternalUrls)
	go func() {
		for itemDoc := range chanItemsDoc {
			go func(doc *goquery.Document) {
				data, multipleItemsUrl, err := parser.getItemData(doc)
				if err != nil {
					log.Fatal(err)
				}
				if len(multipleItemsUrl) != 0 {
					chanInternalUrls <- multipleItemsUrl
				} else {
					chanItemsData <- data
				}
			}(itemDoc)
		}
	}()
	chanInternalDocs := make(chan *goquery.Document, 30)
	defer close(chanInternalDocs)
	go func() {
		for internalUrls := range chanInternalUrls {
			go func(urls []string) {
				for _, internalItemUrl := range urls {
					internalItemDoc, err := parser.getItemDocument(internalItemUrl)
					if err != nil {
						log.Fatal(err)
					}
					chanInternalDocs <- internalItemDoc
				}
			}(internalUrls)
		}
	}()
	go func() {
		for internalDoc := range chanInternalDocs {
			go func(doc *goquery.Document) {
				data, _, err := parser.getItemData(doc)
				if err != nil {
					log.Fatal(err)
				}
				chanItemsData <- data
			}(internalDoc)
		}
	}()

	// form items batch
	itemsBatch := make([]*store.ItemData, 0)
	for {
		/*
			for i := 0; i < 25 || len(chanItemsData) != 0; i++ {
				addItem := <-chanItemsData
				if !common.BatchContains(itemsBatch, addItem) {
					itemsBatch = append(itemsBatch, addItem)
				} else {
					i--
					continue
				}
			}
		*/
		// console out
		common.PrettyPrint(<-chanItemsData)
		/*
			if err := client.WriteBatch(itemsBatch); err != nil {
				log.Fatal(err)
			}
		*/
		itemsBatch = itemsBatch[:0]
	}
}