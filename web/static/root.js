function updateButtons() {
    var delay = 1000;
    $.ajax({
	dataType: "json",
	url: "api/status",
	success: function(data) {
	    $(".scan-button").each(function(){
		if (data["State"] == "IDLE") {
		    $(this).removeAttr("disabled");
		} else {
		    $(this).attr("disabled", "disabled");
		}
	    });
	},
	complete: function() {
	    setTimeout(updateButtons, delay);
	},
    });
}
setTimeout(updateButtons, 100);
