package dtos

type ExcelParserManualUploadRequest struct {
	JobId        uint64  `form:"job_id" json:"job_id" validate:"required"`
	SupplierName string  `form:"supplier_name" json:"supplier_name"`
	SupplierId   *uint64 `form:"supplier_id" json:"supplier_id"`
}

func (this *ExcelParserManualUploadRequest) HasSupplier() bool {
	return this.SupplierId != nil && *this.SupplierId > 0
}
