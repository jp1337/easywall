var countdown = document.getElementById("countdown");

if (countdown != null) {
  var accepttime = document.getElementById("accepttime");
  if (accepttime != null) {
    accepttime = accepttime.getAttribute("data");
  }

  var target = new Date();
  target.setSeconds(target.getSeconds() + accepttime);
  target = target.getTime();

  var seconds, now, distance;

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
