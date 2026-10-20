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
		service := "THG can help with warehousing and fulfillment"
		if kind == serviceExpress {
			service = "THG can check options for international shipping"
			if destEN != "" {
				service += " to " + destEN
			}
		}
		return "Hi " + name + ", " + service + ". Could you share " + joinWithAnd(missing, "and") + " so we can suggest the right option?"
	}
	service := "bên mình có dịch vụ kho và fulfillment"
	if kind == serviceExpress {
		service = "bên mình có thể kiểm tra phương án vận chuyển quốc tế"
		if destVI != "" {
			service += " đi " + destVI
		}
	}
	return leadSalutation(author) + ", " + service + ". Bạn cho mình xin " + joinWithAnd(missing, "và") + " để tư vấn phương án phù hợp nhé?"
}
