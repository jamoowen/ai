package tools

func NewCalculatorTool() *Tool {
	t:= new(Tool)
	t.Name = "Calculator"
	t.Description = "A calculator that can be used to perform mathematical operations on two different numbers"
	t.Parameters = JSONSchema{
		"firstNumber": "The first"
	}
}

