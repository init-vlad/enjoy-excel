package laravel_client

type ProductField string

const (
	ProductFieldName        ProductField = "name"
	ProductFieldPrice       ProductField = "price"
	ProductFieldSlug        ProductField = "slug"
	ProductFieldDescription ProductField = "description"
	ProductFieldQuantity    ProductField = "quantity"
	ProductFieldBrandID     ProductField = "brand_id"
	ProductFieldSKU         ProductField = "sku"
	ProductFieldDiscount    ProductField = "discount"
	ProductFieldCategoryID  ProductField = "category_id"
	ProductFieldIsPopular   ProductField = "is_popular"
	ProductFieldUnknown     ProductField = "unknown"
)

func (pf ProductField) String() string {
	return string(pf)
}

func (pf ProductField) IsValid() bool {
	switch pf {
	case ProductFieldName, ProductFieldPrice, ProductFieldSlug, ProductFieldDescription,
		ProductFieldQuantity, ProductFieldBrandID, ProductFieldSKU, ProductFieldDiscount,
		ProductFieldCategoryID, ProductFieldIsPopular, ProductFieldUnknown:
		return true
	default:
		return false
	}
}

var allProductFields = []ProductField{
	ProductFieldName,
	ProductFieldPrice,
	ProductFieldSlug,
	ProductFieldDescription,
	ProductFieldQuantity,
	ProductFieldBrandID,
	ProductFieldSKU,
	ProductFieldDiscount,
	ProductFieldCategoryID,
	ProductFieldIsPopular,
	ProductFieldUnknown,
}

func AllProductFields() []ProductField {
	return allProductFields
}

var allProductFieldMap = map[ProductField]struct{}{
	ProductFieldName:        {},
	ProductFieldPrice:       {},
	ProductFieldSlug:        {},
	ProductFieldDescription: {},
	ProductFieldQuantity:    {},
	ProductFieldBrandID:     {},
	ProductFieldSKU:         {},
	ProductFieldDiscount:    {},
	ProductFieldCategoryID:  {},
	ProductFieldIsPopular:   {},
	ProductFieldUnknown:     {},
}

func AllProductFieldMap() map[ProductField]struct{} {
	return allProductFieldMap
}
