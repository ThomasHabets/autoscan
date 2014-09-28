function updateStatus() {
    var delay = 1000;
    $.ajax({
	dataType: "json",
	url: "api/status",
	success: function(data) {
	    var o = $("#status-div");
	    classes = "msg"
	    console.log(data)
	    if (data["State"] == "IDLE") {
		if (data["LastFail"] != "") {
		    classes += " fail";
		    o.text("Last scan FAILED: " + data["LastFail"]);
		} else {
		    classes += " success";
		    o.text("Last scan succeeded");
		}
	    } else {
		o.text(data["State"] + "...");
		classes += " active"
		delay = 500;
	    }
	    o.removeClass();
	    o.addClass(classes);
	},
	complete: function() {
	    setTimeout(updateStatus, delay);
	},
    });
}
setTimeout(updateStatus, 100);
