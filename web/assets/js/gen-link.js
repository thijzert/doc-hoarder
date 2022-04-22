
(async () => {
	let me = null;
	document.querySelectorAll("script").forEach((x) => {
		if ( /\/gen-link.js$/.test(x.src) ) {
			me = x;
		}
	});

	let jsu = new URL("./flatten.js", location.href);
	jsu.searchParams.append("base", (new URL("./", location.href)).toString())
	
	let js = await fetch(jsu);
	let jsb = await js.text();
	jsb = "javascript:" + jsb; // jsb = "javascript:" + encodeURIComponent(jsb);

	let p = document.createElement("P");
	if ( me === null ) {
		document.body.appendChild(p);
	} else {
		me.parentNode.insertBefore(p, me.nextChild);
	}

	let a = document.createElement("A");
	a.setAttribute("onclick", "return false;");
	a.href = jsb;
	a.innerText = "HOARD";
	p.appendChild(a);
	p.appendChild(document.createTextNode(" - " + jsb.length + " bytes"));
	document.write(tpl.innerHTML);
})()

