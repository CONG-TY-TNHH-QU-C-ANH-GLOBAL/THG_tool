package facebook

import "strings"

// A named destination is the customer's requirement, not proof that a line
// accepts their goods. The draft offers to check options until a quote exists.
func serviceReplyText(kind, author, destVI, destEN string, missing []string, english bool) string {
	if english {
		name := strings.TrimSpace(author)
		if name == "" {
			name = "there"
		}
		service := "warehousing and fulfillment"
		if kind == serviceExpress {
			service = "international shipping"
			if destEN != "" {
				service += " to " + destEN
			}
		}
		return "Hi " + name + ", for " + service + ", could you share " + joinWithAnd(missing, "and") + " so we can check suitable options?"
	}
	service := "dịch vụ kho và fulfillment"
	if kind == serviceExpress {
		service = "vận chuyển quốc tế"
		if destVI != "" {
			service += " đi " + destVI
		}
	}
	return leadSalutation(author) + ", về " + service + ", bạn cho mình xin " + joinWithAnd(missing, "và") + " để kiểm tra phương án phù hợp nhé?"
}
