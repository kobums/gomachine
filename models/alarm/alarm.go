package alarm


type Type int

const (
    _ Type  = iota

    Notice
    Alarm
    Comment
    Match
)

var Types = []string{ "", "공지", "알람", "게시판", "경기결과" }

type Status int

const (
    _ Status  = iota

    Success
    Fail
)

var Statuss = []string{ "", "성공", "실패" }



func GetType(value Type) string {
    i := int(value)
    if i <= 0 || i >= len(Types) {
        return ""
    }
     
    return Types[i]
}

func GetStatus(value Status) string {
    i := int(value)
    if i <= 0 || i >= len(Statuss) {
        return ""
    }
     
    return Statuss[i]
}

