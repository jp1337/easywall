// set the date we're counting down to
var target = new Date();
var accepttime = document.getElementById("accepttime").getAttribute("data");
target.setSeconds(target.getSeconds() + accepttime);
var target = target.getTime();

// variables for time units
var seconds, now, distance;

// get tag element
var countdown = document.getElementById("countdown");

// update the tag with id "countdown" every 1 second
if (countdown != null) {
  setInterval(function () {
    now = new Date().getTime();
    distance = target - now;
    seconds = Math.floor((distance % (1000 * accepttime)) / 1000);
    countdown.innerHTML = seconds;
  }, 1000);

  setInterval(function () {
    var form = document.createElement("form");
    form.method = "post";
    form.action = "/apply-save";
    var input = document.createElement("input");
    input.type = "text";
    input.name = "step_timeout";
    form.appendChild(input);
    document.body.appendChild(form);
    form.submit();
  }, accepttime * 1000);
}
