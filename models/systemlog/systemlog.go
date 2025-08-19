package systemlog


type Type int

const (
    _ Type  = iota

    Login
    Crawling
)

var Types = []string{ "", "로그인", "크롤링" }



func GetType(value Type) string {
    i := int(value)
    if i <= 0 || i >= len(Types) {
        return ""
    }
     
    return Types[i]
}

