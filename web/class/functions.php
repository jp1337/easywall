<?php

function getCurrentVersion() {
	global $input;
	$version = $input->sessionGet('currentVersion');
	if (!isset($version) || $version === null || $version === false) {
		$version = file_get_contents('../.version');
		$input->sessionSet('currentVersion', $version);
	}
	return $version;
}

function getLatestVersion() {
	global $input;
	$version = $input->sessionGet('latestVersion');
	if (!isset($version) || $version === null || $version === false) {
		$version = file_get_contents('https://raw.githubusercontent.com/KingJP/EasyWall/master/.version');
		$input->sessionSet('latestVersion', $version);
	}
	return $version;
}

function getLastCommit() {
	global $input;
	$commit = $input->sessionGet('lastCommit');
	if (!isset($commit) || $commit === null || $commit === false) {
		$options  = array('http' => array('user_agent' => 'EasyWall Firewall by KingJP'));
		$context  = stream_context_create($options);
		$raw = file_get_contents('https://api.github.com/repos/kingjp/easywall/commits/master', false, $context);
		$commit = json_decode($raw);
		$input->sessionSet('lastCommit', $commit);
	}
	return $commit;
}

function getLatestCommitSha() {
	$json = getLastCommit();
	return $json->sha;
}

function getLatestCommitDate() {
	$json = getLastCommit();
	$datetime = strtotime($json->commit->author->date);
	return humanTiming($datetime);
}

function getReqUri() {
	global $input;
	return basename($input->server('REQUEST_URI'));
}

function humanTiming($time)
{
    $time = time() - $time; // to get the time since that moment
    $time = ($time<1)? 1 : $time;
    $tokens = array (
        31536000 => 'year',
        2592000 => 'month',
        604800 => 'week',
        86400 => 'day',
        3600 => 'hour',
        60 => 'minute',
        1 => 'second'
    );

    foreach ($tokens as $unit => $text) {
        if ($time < $unit) continue;
        $numberOfUnits = floor($time / $unit);
        return $numberOfUnits.' '.$text.(($numberOfUnits>1)?'s':'');
    }
}

function startsWith($haystack, $needle)
{
     $length = strlen($needle);
     return (substr($haystack, 0, $length) === $needle);
}

function endsWith($haystack, $needle)
{
    $length = strlen($needle);
    if ($length == 0) {
        return true;
    }

    return (substr($haystack, -$length) === $needle);
}

?>