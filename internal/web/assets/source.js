(function () {
var root = document.querySelector('.irl-source');
var text = document.querySelector('.irl-text');
var path = window.location.pathname.replace(/\/$/, '');
var events = new EventSource(path + '/events');
events.onmessage = function (event) {
 var value = JSON.parse(event.data);
 text.textContent = value;
 root.classList.toggle('irl-empty', value === '');
};
}());
