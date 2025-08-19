package setting


type Type int

const (
    _ Type  = iota

    NumberType
    StringType
    SelectType
    WeekType
)

var Types = []string{ "", "숫자", "문자", "선택", "요일" }



func GetType(value Type) string {
    i := int(value)
    if i <= 0 || i >= len(Types) {
        return ""
    }
     
    return Types[i]
}

