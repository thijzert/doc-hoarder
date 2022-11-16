
(async () => {
	const rgb = /^rgb\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)$/i;
	let currentTheme = "dark";
	let m = window.getComputedStyle(document.body).backgroundColor.match(rgb);
	if ( m ) {
		if ( (m[1]-0) + (m[2]-0) + (m[3]-0) < 384 ) {
			currentTheme = "light";
		}
	}

	const bt = document.getElementById("btn-toggle-theme");

	const toggleTheme = () => {
		document.documentElement.classList.remove(currentTheme);
		bt.textContent = `Switch to ${currentTheme} theme`;
		if ( currentTheme == "light" ) {
			currentTheme = "dark";
		} else {
			currentTheme = "light";
		}
		document.documentElement.classList.add(currentTheme);
	};
	bt.onclick = toggleTheme;
	toggleTheme();

})()

