import { JSON } from 'json-as'


@json
class EmailTemplateVal {
	unrolled: boolean;
	to: string[];
}

@json
class EmailTemplate {
	id!: string;
	model!: string;
	val!: EmailTemplateVal;
}

export function validate(oldJ: string, nuwJ: string): string {

	var nuw = JSON.parse<EmailTemplate>(nuwJ)
	if (nuw.model != "com.example.EmailTemplate") {
		return "incorrect model passed to validator";
	}

	if (oldJ != "") {
		var old = JSON.parse<EmailTemplate>(oldJ)
		if (old.val.unrolled == true) {
			return "unrolled template cannot be changed";
		}
	}

	for (var i = 0; i < nuw.val.to.length; i++) {
		const atSymbols = nuw.val.to[i].split('@');
		if (atSymbols.length !== 2) {
			return 'Email must contain exactly one @ symbol'
		}
	}


	return "";
}
