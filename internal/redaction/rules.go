package redaction

import (
	"regexp"

	"archive-review/internal/domain"
)

type Rule struct {
	ID         string
	Category   domain.FindingCategory
	Confidence domain.Confidence
	Basis      string
	Pattern    *regexp.Regexp
}

func DefaultRules() []Rule {
	return []Rule{
		{ID: "identity.cn_id.v1", Category: domain.CategoryIdentity, Confidence: domain.ConfidenceHigh,
			Basis: "匹配 18 位中华人民共和国公民身份号码格式", Pattern: regexp.MustCompile(`(?:^|[^0-9])([1-9][0-9]{5}(?:19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx])(?:[^0-9Xx]|$)`)},
		{ID: "contact.mobile_cn.v1", Category: domain.CategoryContact, Confidence: domain.ConfidenceHigh,
			Basis: "匹配中国大陆 11 位移动电话号码格式", Pattern: regexp.MustCompile(`(?:^|[^0-9])(1[3-9][0-9]{9})(?:[^0-9]|$)`)},
		{ID: "contact.email.v1", Category: domain.CategoryContact, Confidence: domain.ConfidenceHigh,
			Basis: "匹配电子邮箱地址格式", Pattern: regexp.MustCompile(`(?i)([a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+)`)},
		{ID: "identity.named_person.v1", Category: domain.CategoryIdentity, Confidence: domain.ConfidenceMedium,
			Basis: "匹配带姓名标签的中文姓名", Pattern: regexp.MustCompile(`(?:姓名|联系人)[：:\s]*([\p{Han}·]{2,8})`)},
		{ID: "restricted.archive_number.v1", Category: domain.CategoryRestricted, Confidence: domain.ConfidenceMedium,
			Basis: "匹配带标签的受限档案或案件编号", Pattern: regexp.MustCompile(`(?:档案编号|案件编号|内部编号)[：:\s]*([A-Za-z0-9][A-Za-z0-9/_-]{5,31})`)},
		{ID: "contact.landline.v1", Category: domain.CategoryContact, Confidence: domain.ConfidenceMedium,
			Basis: "匹配带区号的固定电话号码格式", Pattern: regexp.MustCompile(`(?:^|[^0-9])((?:0[1-9][0-9]{1,2}-)[0-9]{7,8})(?:[^0-9]|$)`)},
	}
}
