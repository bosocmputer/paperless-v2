package api

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/phpdave11/gofpdf"
)

//go:embed fonts/Sarabun-Regular.ttf
var internalThaiFont []byte

//go:embed fonts/NotoSansLao.ttf
var internalLaoFont []byte

var internalPDFSlots = make(chan struct{}, 2)

const (
	internalFontThai = "InternalThai"
	internalFontLao  = "InternalLao"
)

func renderInternalDocumentPDF(document models.InternalDocument) ([]byte, int, error) {
	internalPDFSlots <- struct{}{}
	defer func() { <-internalPDFSlots }()

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 9, 10)
	pdf.SetAutoPageBreak(false, 8)
	pdf.AddUTF8FontFromBytes(internalFontThai, "", internalThaiFont)
	pdf.AddUTF8FontFromBytes(internalFontLao, "", internalLaoFont)
	pdf.SetTitle(document.MasterName+" "+document.DocumentNo, true)
	pdf.SetAuthor("PaperLess", true)

	items := document.Items
	if len(items) == 0 {
		return nil, 0, fmt.Errorf("internal document must contain at least one item")
	}
	pageCount := int(math.Ceil(float64(len(items)) / 15.0))
	if pageCount < 1 {
		pageCount = 1
	}
	if pageCount > 25 {
		return nil, 0, fmt.Errorf("internal document exceeds 25 PDF pages")
	}
	for page := 0; page < pageCount; page++ {
		pdf.AddPage()
		isFirst := page == 0
		isLast := page == pageCount-1
		drawInternalHeader(pdf, document, isFirst, page+1, pageCount)
		start := page * 15
		end := start + 15
		if end > len(items) {
			end = len(items)
		}
		drawInternalItemsTable(pdf, items[start:end], start, isLast)
		if isLast {
			drawInternalSummary(pdf, document)
		}
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, 0, err
	}
	if pdf.Error() != nil {
		return nil, 0, pdf.Error()
	}
	return output.Bytes(), pageCount, nil
}

func drawInternalHeader(pdf *gofpdf.Fpdf, document models.InternalDocument, full bool, pageNo, pageCount int) {
	left, width := 10.0, 190.0
	company := document.CompanySnapshot
	drawInternalMixedCell(pdf, left, 9, width, 7, company.DisplayName, 13, "C")
	companyLine := joinNonEmpty(" · ", company.Address1, company.Address2)
	drawInternalMixedCell(pdf, left, 16, width, 4.5, companyLine, 8, "C")
	contact := []string{}
	if company.TelephoneNumber != "" {
		contact = append(contact, "โทร. "+company.TelephoneNumber)
	}
	if company.TaxNumber != "" {
		contact = append(contact, "เลขประจำตัวผู้เสียภาษี "+company.TaxNumber)
	}
	if branch := internalBranchLabel(company); branch != "" {
		contact = append(contact, branch)
	}
	pdf.SetFont(internalFontThai, "", 7.5)
	pdf.SetXY(left, 20.5)
	pdf.CellFormat(width, 4.5, strings.Join(contact, "  |  "), "", 1, "C", false, 0, "")
	pdf.SetFont(internalFontThai, "", 14)
	pdf.SetX(left)
	pdf.CellFormat(width, 8, document.MasterName, "", 1, "C", false, 0, "")
	pdf.SetFont(internalFontThai, "", 8)
	pdf.SetXY(145, 9)
	pdf.CellFormat(55, 5, "เลขที่  "+document.DocumentNo, "", 1, "R", false, 0, "")
	pdf.SetX(145)
	pdf.CellFormat(55, 5, "วันที่  "+formatInternalDate(document.DocumentDate), "", 1, "R", false, 0, "")
	if pageCount > 1 {
		pdf.SetX(145)
		pdf.CellFormat(55, 5, fmt.Sprintf("หน้า %d/%d", pageNo, pageCount), "", 1, "R", false, 0, "")
	}

	if !full {
		pdf.SetY(36)
		return
	}
	y := 38.0
	pdf.SetFont(internalFontThai, "", 9)
	drawLabeledCell(pdf, left, y, 110, 8, "ชื่อผู้ขอเบิก", document.RequesterName)
	drawLabeledCell(pdf, left+110, y, 80, 8, "วันที่ต้องการใช้เงิน", formatInternalDate(document.RequiredDate))
	y += 8
	drawLabeledCell(pdf, left, y, 80, 8, "ตำแหน่ง", document.PositionName)
	drawLabeledCell(pdf, left+80, y, 110, 8, "ส่วนงาน/ฝ่าย/แผนก", document.DepartmentName)
	y += 8
	pdf.Rect(left, y, width, 22, "D")
	pdf.SetXY(left+2, y+1)
	pdf.SetFont(internalFontThai, "", 8)
	pdf.CellFormat(width-4, 5, "วัตถุประสงค์", "", 1, "L", false, 0, "")
	pdf.SetXY(left+2, y+6)
	drawInternalMixedMultiline(pdf, left+2, y+6, width-4, 5, 3, document.Purpose, 9)
	pdf.SetY(y + 24)
}

func drawLabeledCell(pdf *gofpdf.Fpdf, x, y, width, height float64, label, value string) {
	pdf.Rect(x, y, width, height, "D")
	pdf.SetXY(x+2, y+1.2)
	pdf.SetFont(internalFontThai, "", 7)
	pdf.CellFormat(width*0.32, 5, label, "", 0, "L", false, 0, "")
	drawInternalMixedCell(pdf, x+2+width*0.32, y+1.2, width*0.66-3, 5, value, 9, "L")
}

func drawInternalItemsTable(pdf *gofpdf.Fpdf, items []models.InternalDocumentItem, offset int, finalPage bool) {
	y := pdf.GetY()
	if y < 38 {
		y = 38
	}
	left := 10.0
	cols := []float64{14, 136, 40}
	pdf.SetFillColor(242, 244, 247)
	pdf.SetFont(internalFontThai, "", 8.5)
	pdf.SetXY(left, y)
	pdf.CellFormat(cols[0], 8, "ลำดับ", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols[1], 8, "รายการ", "1", 0, "C", true, 0, "")
	pdf.CellFormat(cols[2], 8, "จำนวนเงินประมาณ (บาท)", "1", 1, "C", true, 0, "")
	y += 8
	rowCount := len(items)
	if finalPage && rowCount < 5 {
		rowCount = 5
	}
	for i := 0; i < rowCount; i++ {
		pdf.SetXY(left, y)
		if i < len(items) {
			item := items[i]
			pdf.SetFont(internalFontThai, "", 8.5)
			pdf.CellFormat(cols[0], 9, strconv.Itoa(offset+i+1), "1", 0, "C", false, 0, "")
			pdf.Rect(left+cols[0], y, cols[1], 9, "D")
			drawInternalMixedCell(pdf, left+cols[0]+1.2, y, cols[1]-2.4, 9, item.Description, 8.5, "L")
			pdf.SetXY(left+cols[0]+cols[1], y)
			pdf.SetFont(internalFontThai, "", 8.5)
			pdf.CellFormat(cols[2], 9, formatInternalAmount(item.Amount), "1", 1, "R", false, 0, "")
		} else {
			pdf.CellFormat(cols[0], 9, "", "1", 0, "C", false, 0, "")
			pdf.CellFormat(cols[1], 9, "", "1", 0, "L", false, 0, "")
			pdf.CellFormat(cols[2], 9, "", "1", 1, "R", false, 0, "")
		}
		y += 9
	}
	pdf.SetY(y)
}

func drawInternalSummary(pdf *gofpdf.Fpdf, document models.InternalDocument) {
	left, width := 10.0, 190.0
	y := pdf.GetY()
	pdf.SetFont(internalFontThai, "", 9)
	pdf.SetXY(left, y)
	pdf.CellFormat(150, 9, "("+thaiBahtText(document.TotalAmount)+")", "1", 0, "C", false, 0, "")
	pdf.SetFont(internalFontThai, "", 9.5)
	pdf.CellFormat(40, 9, "รวม  "+formatInternalAmount(document.TotalAmount), "1", 1, "R", false, 0, "")
	y += 9
	pdf.SetXY(left, y)
	pdf.CellFormat(width, 7, "การอนุมัติเอกสาร", "1", 1, "C", true, 0, "")
	y += 7
	cellWidth := width / 2
	for i, label := range []string{"ผู้ขอเบิก", "ผู้อนุมัติ/ตรวจสอบ"} {
		x := left + float64(i)*cellWidth
		pdf.Rect(x, y, cellWidth, 34, "D")
		pdf.SetXY(x, y+1)
		pdf.CellFormat(cellWidth, 6, label, "", 0, "C", false, 0, "")
		pdf.SetXY(x, y+27)
		pdf.CellFormat(cellWidth, 5, "วันที่ ______ / ______ / ______", "", 0, "C", false, 0, "")
	}
	y += 34
	pdf.SetTextColor(205, 35, 45)
	pdf.SetFont(internalFontThai, "", 8.5)
	pdf.SetXY(left, y+1)
	pdf.CellFormat(width, 6, "*ให้ผู้เบิกเงินเคลียร์สำรองจ่ายภายใน 15 วัน นับจากวันรับเงิน", "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

func thaiBahtText(value string) string {
	cents, err := parseAmountCents(value)
	if err != nil || cents < 0 {
		return ""
	}
	baht := cents / 100
	satang := cents % 100
	result := thaiIntegerText(baht) + "บาท"
	if satang == 0 {
		return result + "ถ้วน"
	}
	return result + thaiIntegerText(satang) + "สตางค์"
}

func thaiIntegerText(value int64) string {
	if value == 0 {
		return "ศูนย์"
	}
	if value >= 1_000_000 {
		high := value / 1_000_000
		low := value % 1_000_000
		return thaiIntegerText(high) + "ล้าน" + func() string {
			if low > 0 {
				return thaiIntegerText(low)
			}
			return ""
		}()
	}
	digits := []string{"ศูนย์", "หนึ่ง", "สอง", "สาม", "สี่", "ห้า", "หก", "เจ็ด", "แปด", "เก้า"}
	units := []string{"", "สิบ", "ร้อย", "พัน", "หมื่น", "แสน"}
	text := ""
	positions := 0
	for n := value; n > 0; n /= 10 {
		positions++
	}
	for pos := positions - 1; pos >= 0; pos-- {
		power := int64(math.Pow10(pos))
		digit := int((value / power) % 10)
		if digit == 0 {
			continue
		}
		word := digits[digit]
		if pos == 1 {
			if digit == 1 {
				word = ""
			}
			if digit == 2 {
				word = "ยี่"
			}
		} else if pos == 0 && digit == 1 && value > 10 {
			word = "เอ็ด"
		}
		text += word + units[pos]
	}
	return text
}

func parseAmountCents(value string) (int64, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount")
	}
	frac := "00"
	if len(parts) == 2 {
		frac = (parts[1] + "00")[:2]
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	decimal, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	return whole*100 + decimal, nil
}

func formatInternalAmount(value string) string {
	cents, err := parseAmountCents(value)
	if err != nil {
		return value
	}
	whole := cents / 100
	frac := cents % 100
	raw := strconv.FormatInt(whole, 10)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "," + raw[i:]
	}
	return fmt.Sprintf("%s.%02d", raw, frac)
}

func formatInternalDate(value string) string {
	parts := strings.Split(value, "-")
	if len(parts) != 3 {
		return value
	}
	return parts[2] + "/" + parts[1] + "/" + parts[0]
}

type internalTextRun struct {
	font string
	text string
}

func drawInternalMixedCell(pdf *gofpdf.Fpdf, x, y, width, height float64, value string, fontSize float64, align string) {
	value = fitInternalTextWidth(pdf, strings.TrimSpace(value), width, fontSize)
	runs := internalTextRuns(value)
	totalWidth := internalTextWidth(pdf, runs, fontSize)
	startX := x
	switch align {
	case "C":
		startX += math.Max(0, (width-totalWidth)/2)
	case "R":
		startX += math.Max(0, width-totalWidth)
	}
	for _, run := range runs {
		pdf.SetFont(run.font, "", fontSize)
		runWidth := pdf.GetStringWidth(run.text)
		pdf.SetXY(startX, y)
		pdf.CellFormat(runWidth, height, run.text, "", 0, "L", false, 0, "")
		startX += runWidth
	}
}

func drawInternalMixedMultiline(pdf *gofpdf.Fpdf, x, y, width, lineHeight float64, maxLines int, value string, fontSize float64) {
	lines := wrapInternalText(pdf, strings.TrimSpace(value), width, fontSize, maxLines)
	for i, line := range lines {
		drawInternalMixedCell(pdf, x, y+float64(i)*lineHeight, width, lineHeight, line, fontSize, "L")
	}
}

func internalTextRuns(value string) []internalTextRun {
	runs := []internalTextRun{}
	for _, r := range value {
		font := internalFontThai
		if unicode.In(r, unicode.Lao) {
			font = internalFontLao
		}
		if len(runs) == 0 || runs[len(runs)-1].font != font {
			runs = append(runs, internalTextRun{font: font, text: string(r)})
		} else {
			runs[len(runs)-1].text += string(r)
		}
	}
	return runs
}

func internalTextWidth(pdf *gofpdf.Fpdf, runs []internalTextRun, fontSize float64) float64 {
	width := 0.0
	for _, run := range runs {
		pdf.SetFont(run.font, "", fontSize)
		width += pdf.GetStringWidth(run.text)
	}
	return width
}

func fitInternalTextWidth(pdf *gofpdf.Fpdf, value string, width, fontSize float64) string {
	if internalTextWidth(pdf, internalTextRuns(value), fontSize) <= width {
		return value
	}
	runes := []rune(value)
	for len(runes) > 3 {
		candidate := strings.TrimSpace(string(runes[:len(runes)-3])) + "..."
		if internalTextWidth(pdf, internalTextRuns(candidate), fontSize) <= width {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return "..."
}

func wrapInternalText(pdf *gofpdf.Fpdf, value string, width, fontSize float64, maxLines int) []string {
	if value == "" || maxLines < 1 {
		return nil
	}
	lines := []string{}
	current := ""
	remaining := []rune(value)
	for len(remaining) > 0 && len(lines) < maxLines {
		r := remaining[0]
		remaining = remaining[1:]
		if r == '\n' {
			lines = append(lines, strings.TrimSpace(current))
			current = ""
			continue
		}
		candidate := current + string(r)
		if current != "" && internalTextWidth(pdf, internalTextRuns(candidate), fontSize) > width {
			lines = append(lines, strings.TrimSpace(current))
			current = string(r)
			continue
		}
		current = candidate
	}
	if len(lines) < maxLines && strings.TrimSpace(current) != "" {
		lines = append(lines, strings.TrimSpace(current))
		current = ""
	}
	if len(remaining) > 0 || strings.TrimSpace(current) != "" {
		last := len(lines) - 1
		if last < 0 {
			return []string{"..."}
		}
		lines[last] = fitInternalTextWidth(pdf, strings.TrimSpace(lines[last])+"...", width, fontSize)
	}
	return lines
}

func joinNonEmpty(separator string, values ...string) string {
	items := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, strings.TrimSpace(value))
		}
	}
	return strings.Join(items, separator)
}

func internalBranchLabel(company models.InternalDocumentCompanySnapshot) string {
	if company.BranchCode != "" {
		return "สาขา " + company.BranchCode
	}
	if company.BranchStatus == 0 || company.BranchType == 0 {
		return "สำนักงานใหญ่"
	}
	return ""
}
