<?php
class Rules
{
	protected $rules;
	protected $filename;
	
	public function __construct($filename) {
		$this->rules = array();
		$this->filename = $filename;
		
		if ($this->readRules($filename) === false) {
			return false;
		}
		return true;
    }
	
	protected function readRules($filename) {
		$content = file($filename);
	
		foreach ($content as $line) {
			if (!ctype_space($line) && $line != '') {
				if (substr( $line, 0, 1 ) != "#") {
					array_push($this->rules, trim($line));
				}
			}
		}
		
		if (count($this->rules) > -1) {
			return true;
		}
		return false;
	}
	
	protected function writeRules($filename) {
		$currContent = file($filename);
		$content = "";
		
		foreach ($currContent as $line) {
			if (!ctype_space($line) && $line != '') {
				if (substr( $line, 0, 1 ) == "#") {
					$content .= $line;
				}
			}
		}
	
		foreach ($this->rules as $value) {
			$content .= $value . "\n";
		}
		
		if (file_put_contents($filename, $content) != false) {
			return true;
		}
		return false;
	}
	
	public function getAll() {
		if (isset($this->rules)) {
			try {
				array_multisort($this->rules, SORT_ASC, SORT_NUMERIC);
			} catch (Exception $e) {
				asort($this->rules);
			}
			return $this->rules;
		}
		return false;
    }
	
	public function remove($ipadr) {
		if (in_array($ipadr, $this->rules)) {
			$index = array_search($ipadr, $this->rules);
			unset($this->rules[$index]);
			return $this->writeRules($this->filename);
		}
		return false;
	}
	
	public function add($ipadr) {
		if (!in_array($ipadr, $this->rules)) {
			array_push($this->rules, $ipadr);
			return $this->writeRules($this->filename);
		}
		return false;
	}
}
?>
