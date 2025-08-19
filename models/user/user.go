package user


type Level int

const (
    _ Level  = iota

    Normal
    Manager
    Admin
    Superadmin
    Rootadmin
)

var Levels = []string{ "", "일반", "팀장", "관리자", "승인관리자", "전체관리자" }

type Use int

const (
    _ Use  = iota

    Use
    Notuse
)

var Uses = []string{ "", "사용", "사용안함" }

type Type int

const (
    _ Type  = iota

    Normal
    Kakao
    Naver
    Google
    Apple
)

var Types = []string{ "", "일반", "카카오", "네이버", "구글", "애플" }

type Role int

const (
    _ Role  = iota

    Supervisor
    Coach
    Parent
    Player
    Use
    Normal
)

var Roles = []string{ "", "감독", "코치", "학부모", "현역선수", "동호회", "일반인" }



func GetLevel(value Level) string {
    i := int(value)
    if i <= 0 || i >= len(Levels) {
        return ""
    }
     
    return Levels[i]
}

func GetUse(value Use) string {
    i := int(value)
    if i <= 0 || i >= len(Uses) {
        return ""
    }
     
    return Uses[i]
}

func GetType(value Type) string {
    i := int(value)
    if i <= 0 || i >= len(Types) {
        return ""
    }
     
    return Types[i]
}

func GetRole(value Role) string {
    i := int(value)
    if i <= 0 || i >= len(Roles) {
        return ""
    }
     
    return Roles[i]
}

