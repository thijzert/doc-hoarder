
(async () => {
	let me = null;
	document.querySelectorAll("script").forEach((x) => {
		if ( /\/gen-link.js$/.test(x.src) ) {
			me = x;
		}
	});

	let p = document.createElement("P");
	if ( me === null ) {
		document.body.appendChild(p);
	} else {
		me.parentNode.insertBefore(p, me.nextChild);
	}

	let q = document.createElement("P");
	p.parentNode.insertBefore(q, p);

	let xpu = new URL("./doc-hoarder.xpi", location.href);
	xpu.searchParams.append("base", (new URL("./", location.href)).toString())
	let a = document.createElement("A");
	a.href = xpu;
	a.innerText = "Download extension";
	q.appendChild(a);


	let jsu = new URL("./flatten.js", location.href);
	jsu.searchParams.append("base", (new URL("./", location.href)).toString())

	let js = await fetch(jsu);
	let jsb = await js.text();
	jsb = "javascript:" + jsb; // jsb = "javascript:" + encodeURIComponent(jsb);


	a = document.createElement("A");
	a.setAttribute("onclick", "return false;");
	a.href = jsb;
	a.innerText = "HOARD";
	p.appendChild(a);
	p.appendChild(document.createTextNode(" - " + jsb.length + " bytes"));
	document.write(tpl.innerHTML);
})()

