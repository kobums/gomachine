package ipblock


type Type int

const (
    _ Type  = iota

    Admin
    Normal
)

var Types = []string{ "", "관리자 접근", "일반 접근" }

type Policy int

const (
    _ Policy  = iota

    Grant
    Deny
)

var Policys = []string{ "", "허용", "거부" }

type Use int

const (
    _ Use  = iota

    Use
    Notuse
)

var Uses = []string{ "", "사용", "사용안함" }



func GetType(value Type) string {
    i := int(value)
    if i <= 0 || i >= len(Types) {
        return ""
    }
     
    return Types[i]
}

func GetPolicy(value Policy) string {
    i := int(value)
    if i <= 0 || i >= len(Policys) {
        return ""
    }
     
    return Policys[i]
}

func GetUse(value Use) string {
    i := int(value)
    if i <= 0 || i >= len(Uses) {
        return ""
    }
     
    return Uses[i]
}

