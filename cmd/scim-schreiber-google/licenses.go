package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Product struct {
	ProductId string `yaml:"productId"`
	SkuId     string `yaml:"skuId"`
}

type productsYaml struct {
	Products map[string]Product `yaml:"products"`
}

type ProductInformation struct {
	Products          map[string]Product
	ReverseProductMap map[string]map[string]string
}

func ProductInformationFromProducts(products map[string]Product) *ProductInformation {
	return &ProductInformation{
		Products:          products,
		ReverseProductMap: reverseProductMap(products),
	}
}

func NewProductInformationFromFile(filename string) *ProductInformation {
	f, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	var products productsYaml

	// Unmarshal our input YAML file into empty Car (var c)
	if err := yaml.Unmarshal(f, &products); err != nil {
		log.Fatal(err)
	}

	return ProductInformationFromProducts(products.Products)
}

func reverseProductMap(productMap map[string]Product) map[string]map[string]string {
	m := make(map[string]map[string]string)

	for skuName, product := range productMap {
		productId := product.ProductId
		skuId := product.SkuId
		if m[productId] == nil {
			m[productId] = make(map[string]string)
		}
		m[productId][skuId] = skuName
	}

	return m
}
