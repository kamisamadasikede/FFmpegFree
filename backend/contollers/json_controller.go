package contollers

import (
	"FFmpegFree/backend/utils"
	"FFmpegFree/backend/vo"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// JsonFormat JSON格式化处理函数
// 将JSON字符串格式化为易读形式或压缩为单行
func JsonFormat(c *gin.Context) {
	var req vo.JsonFormatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, utils.Fail(400, "参数解析失败: "+err.Error()))
		return
	}

	if req.Json == "" {
		c.JSON(400, utils.Fail(400, "JSON字符串不能为空"))
		return
	}

	indent := req.Indent
	if indent <= 0 {
		indent = 4
	}

	var result string
	var err error

	if req.Compact {
		var data interface{}
		if err := json.Unmarshal([]byte(req.Json), &data); err != nil {
			errPos := parseJsonError(req.Json, err.Error())
			c.JSON(200, utils.Success(vo.JsonFormatResponse{
				Error:    "JSON语法错误: " + err.Error(),
				ErrorPos: errPos,
			}))
			return
		}
		resultBytes, err := json.Marshal(data)
		if err != nil {
			c.JSON(500, utils.Fail(500, "压缩失败: "+err.Error()))
			return
		}
		result = string(resultBytes)
	} else {
		var data interface{}
		if err := json.Unmarshal([]byte(req.Json), &data); err != nil {
			errPos := parseJsonError(req.Json, err.Error())
			c.JSON(200, utils.Success(vo.JsonFormatResponse{
				Error:    "JSON语法错误: " + err.Error(),
				ErrorPos: errPos,
			}))
			return
		}
		indentStr := strings.Repeat(" ", indent)
		result, err = formatJson(data, indentStr)
		if err != nil {
			c.JSON(500, utils.Fail(500, "格式化失败: "+err.Error()))
			return
		}
	}

	c.JSON(200, utils.Success(vo.JsonFormatResponse{
		Formatted: result,
	}))
}

// JsonCompare JSON比对处理函数
// 比较两个JSON对象的差异
func JsonCompare(c *gin.Context) {
	var req vo.JsonCompareRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, utils.Fail(400, "参数解析失败: "+err.Error()))
		return
	}

	if req.Json1 == "" || req.Json2 == "" {
		c.JSON(400, utils.Fail(400, "两个JSON字符串都不能为空"))
		return
	}

	var data1, data2 interface{}

	if err := json.Unmarshal([]byte(req.Json1), &data1); err != nil {
		errPos := parseJsonError(req.Json1, err.Error())
		c.JSON(200, utils.Success(vo.JsonCompareResponse{
			Error:    "第一个JSON语法错误: " + err.Error(),
			ErrorPos: errPos,
		}))
		return
	}

	if err := json.Unmarshal([]byte(req.Json2), &data2); err != nil {
		errPos := parseJsonError(req.Json2, err.Error())
		c.JSON(200, utils.Success(vo.JsonCompareResponse{
			Error:    "第二个JSON语法错误: " + err.Error(),
			ErrorPos: errPos,
		}))
		return
	}

	diffs := compareValues(data1, data2, "")

	c.JSON(200, utils.Success(vo.JsonCompareResponse{
		Identical:   len(diffs) == 0,
		Differences: diffs,
	}))
}

// JsonValidate JSON验证处理函数
// 验证JSON语法并返回错误位置
func JsonValidate(c *gin.Context) {
	var req vo.JsonValidateRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, utils.Fail(400, "参数解析失败: "+err.Error()))
		return
	}

	if req.Json == "" {
		c.JSON(200, utils.Success(vo.JsonValidateResponse{
			Valid: false,
			Error: "JSON字符串不能为空",
		}))
		return
	}

	var data interface{}
	err := json.Unmarshal([]byte(req.Json), &data)

	if err == nil {
		c.JSON(200, utils.Success(vo.JsonValidateResponse{
			Valid: true,
		}))
		return
	}

	errPos := parseJsonError(req.Json, err.Error())
	c.JSON(200, utils.Success(vo.JsonValidateResponse{
		Valid:    false,
		Error:    err.Error(),
		ErrorPos: errPos,
	}))
}

// formatJson 格式化JSON为带缩进的字符串
func formatJson(data interface{}, indent string) (string, error) {
	result, err := json.MarshalIndent(data, "", indent)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// compareValues 递归比较两个值并返回差异列表
func compareValues(v1, v2 interface{}, path string) []vo.Difference {
	var diffs []vo.Difference

	if v1 == nil && v2 == nil {
		return diffs
	}

	type1 := reflect.TypeOf(v1)
	type2 := reflect.TypeOf(v2)

	if type1 == nil || type2 == nil {
		if type1 != type2 {
			diffs = append(diffs, vo.Difference{
				Type:     "modified",
				Path:     path,
				OldValue: fmt.Sprintf("%v", v1),
				NewValue: fmt.Sprintf("%v", v2),
			})
		}
		return diffs
	}

	if type1 != type2 {
		diffs = append(diffs, vo.Difference{
			Type:     "modified",
			Path:     path,
			OldValue: fmt.Sprintf("%v", v1),
			NewValue: fmt.Sprintf("%v", v2),
		})
		return diffs
	}

	switch val1 := v1.(type) {
	case map[string]interface{}:
		val2 := v2.(map[string]interface{})
		allKeys := make(map[string]bool)
		for k := range val1 {
			allKeys[k] = true
		}
		for k := range val2 {
			allKeys[k] = true
		}

		for k := range allKeys {
			newPath := k
			if path != "" {
				newPath = path + "." + k
			}

			_, in1 := val1[k]
			_, in2 := val2[k]

			if in1 && !in2 {
				diffs = append(diffs, vo.Difference{
					Type:     "removed",
					Path:     newPath,
					OldValue: fmt.Sprintf("%v", val1[k]),
					NewValue: "",
				})
			} else if !in1 && in2 {
				diffs = append(diffs, vo.Difference{
					Type:     "added",
					Path:     newPath,
					OldValue: "",
					NewValue: fmt.Sprintf("%v", val2[k]),
				})
			} else {
				subDiffs := compareValues(val1[k], val2[k], newPath)
				diffs = append(diffs, subDiffs...)
			}
		}

	case []interface{}:
		val2 := v2.([]interface{})
		maxLen := len(val1)
		if len(val2) > maxLen {
			maxLen = len(val2)
		}

		for i := 0; i < maxLen; i++ {
			newPath := fmt.Sprintf("%s[%d]", path, i)

			if i >= len(val1) {
				diffs = append(diffs, vo.Difference{
					Type:     "added",
					Path:     newPath,
					OldValue: "",
					NewValue: fmt.Sprintf("%v", val2[i]),
				})
			} else if i >= len(val2) {
				diffs = append(diffs, vo.Difference{
					Type:     "removed",
					Path:     newPath,
					OldValue: fmt.Sprintf("%v", val1[i]),
					NewValue: "",
				})
			} else {
				subDiffs := compareValues(val1[i], val2[i], newPath)
				diffs = append(diffs, subDiffs...)
			}
		}

	default:
		if v1 != v2 {
			diffs = append(diffs, vo.Difference{
				Type:     "modified",
				Path:     path,
				OldValue: fmt.Sprintf("%v", v1),
				NewValue: fmt.Sprintf("%v", v2),
			})
		}
	}

	return diffs
}

// parseJsonError 解析JSON错误并返回错误位置
func parseJsonError(jsonStr, errMsg string) vo.ErrorPos {
	errPos := vo.ErrorPos{Line: 1, Column: 1}

	lineColRe := regexp.MustCompile(`line (\d+) column (\d+)`)
	matches := lineColRe.FindStringSubmatch(errMsg)
	if len(matches) == 3 {
		fmt.Sscanf(matches[1], "%d", &errPos.Line)
		fmt.Sscanf(matches[2], "%d", &errPos.Column)
		return errPos
	}

	posRe := regexp.MustCompile(`position (\d+)`)
	posMatches := posRe.FindStringSubmatch(errMsg)
	if len(posMatches) == 2 {
		var pos int
		fmt.Sscanf(posMatches[1], "%d", &pos)
		errPos = calculateLineColumn(jsonStr, pos)
		return errPos
	}

	re := regexp.MustCompile(`offset (\d+)`)
	reMatches := re.FindStringSubmatch(errMsg)
	if len(reMatches) == 2 {
		var offset int
		fmt.Sscanf(reMatches[1], "%d", &offset)
		errPos = calculateLineColumn(jsonStr, offset)
		return errPos
	}

	invalidRe := regexp.MustCompile(`invalid character '(.+?)'`)
	invalidMatches := invalidRe.FindStringSubmatch(errMsg)
	if len(invalidMatches) == 2 {
		char := invalidMatches[1]
		errPos = findCharPosition(jsonStr, char)
	}

	missingQuoteRe := regexp.MustCompile(`invalid character.*EOF`)
	if missingQuoteRe.MatchString(errMsg) {
		errPos = findMissingQuotePosition(jsonStr)
	}

	return errPos
}

// calculateLineColumn 根据字节偏移计算行号和列号
func calculateLineColumn(jsonStr string, offset int) vo.ErrorPos {
	if offset > len(jsonStr) {
		offset = len(jsonStr)
	}

	line := 1
	column := 1

	for i := 0; i < offset; i++ {
		if jsonStr[i] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}

	return vo.ErrorPos{Line: line, Column: column}
}

// findCharPosition 查找特定字符的位置
func findCharPosition(jsonStr, char string) vo.ErrorPos {
	for i := 0; i < len(jsonStr); i++ {
		if strings.Contains(string(jsonStr[i]), char) {
			return calculateLineColumn(jsonStr, i)
		}
	}
	return vo.ErrorPos{Line: 1, Column: 1}
}

// findMissingQuotePosition 查找缺失引号的大致位置
func findMissingQuotePosition(jsonStr string) vo.ErrorPos {
	openQuotes := 0
	for i, c := range jsonStr {
		if c == '"' {
			if i > 0 && jsonStr[i-1] != '\\' {
				openQuotes = 1 - openQuotes
			}
		}
	}

	if openQuotes == 1 {
		return calculateLineColumn(jsonStr, len(jsonStr))
	}

	return vo.ErrorPos{Line: 1, Column: 1}
}
